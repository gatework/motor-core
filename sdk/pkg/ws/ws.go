// Package ws manages websocket connections with one owner for every send queue.
package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	// ErrClosed reports that the WebSocket manager has stopped accepting work.
	ErrClosed = errors.New("ws: manager closed")
	// ErrNotRunning reports a manager that has not started its command loop.
	ErrNotRunning = errors.New("ws: manager not running")
	// ErrNoClient reports an unknown connection ID and group.
	ErrNoClient = errors.New("ws: client not found")
	// ErrSlowClient reports a connection removed after its send queue fills.
	ErrSlowClient = errors.New("ws: client send queue full")
)

type client struct {
	id, group string
	conn      *websocket.Conn
	outgoing  chan []byte
}

type commandKind uint8

const (
	commandConnect commandKind = iota + 1
	commandDisconnect
	commandSend
	commandGroup
	commandBroadcast
)

type command struct {
	kind      commandKind
	id, group string
	client    *client
	message   []byte
	reply     chan error
}

type counts struct{ groups, clients int }

// Manager owns a single registration/broadcast loop and its connection workers.
// Construct one per host, Run it under the host's lifecycle, then Shutdown it.
type Manager struct {
	once                sync.Once
	mu                  sync.Mutex
	isRunning, isClosed bool
	commands            chan command
	stop, done          chan struct{}
	stats               atomic.Pointer[counts]
	workers             sync.WaitGroup
	opts                options
}

// connectionSet is mutated only by the manager's command loop.
type connectionSet struct {
	groups  map[string]map[string]*client
	clients int
	changed bool
}

// New constructs a manager with bounded incoming messages and network writes.
func New(opts ...Option) (*Manager, error) {
	m := &Manager{opts: defaultOptions()}
	for _, option := range opts {
		if option == nil {
			return nil, errors.New("ws: option is required")
		}
		if err := option(&m.opts); err != nil {
			return nil, err
		}
	}
	m.init()
	return m, nil
}

// Run owns all registrations and waits for connection workers before returning.
func (m *Manager) Run(ctx context.Context) error {
	m.init()
	m.mu.Lock()
	if m.isClosed {
		m.mu.Unlock()
		return ErrClosed
	}
	if m.isRunning {
		m.mu.Unlock()
		return errors.New("ws: already running")
	}
	m.isRunning = true
	m.mu.Unlock()
	clients := &connectionSet{groups: make(map[string]map[string]*client)}
	defer m.finish(clients)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-m.stop:
			return nil
		case op := <-m.commands:
			err := m.apply(ctx, clients, op)
			clients.publishStats(&m.stats)
			if op.reply != nil {
				op.reply <- err
			}
		}
	}
}

// Connect transfers ownership of conn only when registration succeeds.
func (m *Manager) Connect(ctx context.Context, id, group string, conn *websocket.Conn) error {
	if conn == nil || id == "" {
		return errors.New("ws: connection and id are required")
	}
	return m.submit(ctx, command{kind: commandConnect, id: id, group: group, client: &client{id: id, group: group, conn: conn, outgoing: make(chan []byte, 1024)}})
}

// Send queues a message for one connection.
func (m *Manager) Send(ctx context.Context, id, group string, payload []byte) error {
	data, err := envelope(payload)
	if err != nil {
		return err
	}
	return m.submit(ctx, command{kind: commandSend, id: id, group: group, message: data})
}

// SendGroup queues a message for every connection in a group.
func (m *Manager) SendGroup(ctx context.Context, group string, payload []byte) error {
	data, err := envelope(payload)
	if err != nil {
		return err
	}
	return m.submit(ctx, command{kind: commandGroup, group: group, message: data})
}

// SendAll queues a message for every connection.
func (m *Manager) SendAll(ctx context.Context, payload []byte) error {
	data, err := envelope(payload)
	if err != nil {
		return err
	}
	return m.submit(ctx, command{kind: commandBroadcast, message: data})
}

// Disconnect closes the connection and removes its registrations.
func (m *Manager) Disconnect(ctx context.Context, id, group string) error {
	return m.submit(ctx, command{kind: commandDisconnect, id: id, group: group})
}

// Info returns the current connection counts.
func (m *Manager) Info() map[string]any {
	m.init()
	v := m.stats.Load()
	return map[string]any{"groupLen": v.groups, "clientLen": v.clients, "chanRegisterLen": 0, "chanUnregisterLen": 0, "chanMessageLen": len(m.commands), "chanGroupMessageLen": 0, "chanBroadCastMessageLen": 0}
}

// Shutdown stops admissions and waits for owned connection workers.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.init()
	m.mu.Lock()
	if !m.isClosed {
		m.isClosed = true
		close(m.stop)
		if !m.isRunning {
			close(m.done)
		}
	}
	m.mu.Unlock()
	select {
	case <-m.done:
		return nil
	default:
	}
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Handler accepts connections using the host's origin/subprotocol policy.
func (m *Manager) Handler(upgrader websocket.Upgrader) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		if err = m.Connect(c.Request.Context(), c.Param("id"), c.Param("channel"), conn); err != nil {
			_ = conn.Close()
		}
	}
}

// DisconnectHandler retains the existing HTTP close response shape.
func (m *Manager) DisconnectHandler(c *gin.Context) {
	if err := m.Disconnect(c.Request.Context(), c.Param("id"), c.Param("channel")); err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": "ws close success", "msg": "success"})
}

func (m *Manager) init() {
	m.once.Do(func() {
		if m.opts.maxMessageBytes == 0 {
			m.opts = defaultOptions()
		}
		m.commands = make(chan command, 128)
		m.stop = make(chan struct{})
		m.done = make(chan struct{})
		m.stats.Store(&counts{})
	})
}

func (m *Manager) submit(ctx context.Context, op command) error {
	m.init()
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	running, closed := m.isRunning, m.isClosed
	m.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if !running {
		return ErrNotRunning
	}
	// Once submitted, ownership has transferred to the loop. Waiting for its
	// reply avoids returning an error for a registration that actually succeeded.
	op.reply = make(chan error, 1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stop:
		return ErrClosed
	case m.commands <- op:
	}
	select {
	case err := <-op.reply:
		return err
	case <-m.stop:
		return ErrClosed
	}
}

func (m *Manager) read(ctx context.Context, c *client) {
	defer m.disconnect(ctx, c)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		// Echo retains the existing generic handler behaviour without file/job logic.
		op := command{kind: commandSend, id: c.id, group: c.group, client: c, message: data}
		select {
		case <-m.stop:
			return
		case <-ctx.Done():
			return
		case m.commands <- op:
		}
	}
}
func (m *Manager) write(ctx context.Context, c *client) {
	defer m.disconnect(ctx, c)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case data, ok := <-c.outgoing:
			if !ok {
				return
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(m.opts.writeTimeout)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}
func (m *Manager) disconnect(ctx context.Context, c *client) {
	select {
	case <-m.stop:
	case <-ctx.Done():
	case m.commands <- command{kind: commandDisconnect, id: c.id, group: c.group, client: c}:
	}
}

func (m *Manager) finish(clients *connectionSet) {
	m.mu.Lock()
	if !m.isClosed {
		m.isClosed = true
		close(m.stop)
	}
	m.mu.Unlock()
	for _, group := range clients.groups {
		for _, c := range group {
			clients.remove(c)
		}
	}
	clients.publishStats(&m.stats)
	m.workers.Wait()
	close(m.done)
}

func (m *Manager) apply(ctx context.Context, clients *connectionSet, op command) error {
	switch op.kind {
	case commandConnect:
		c := op.client
		clients.add(c)
		c.conn.SetReadLimit(m.opts.maxMessageBytes)
		m.workers.Go(func() { m.read(ctx, c) })
		m.workers.Go(func() { m.write(ctx, c) })
	case commandDisconnect:
		c := clients.groups[op.group][op.id]
		if c != nil && (op.client == nil || op.client == c) {
			clients.remove(c)
		}
	case commandSend:
		c := clients.groups[op.group][op.id]
		if c == nil || op.client != nil && op.client != c {
			return ErrNoClient
		}
		return clients.send(c, op.message)
	case commandGroup:
		return clients.sendGroup(op.group, op.message)
	case commandBroadcast:
		var err error
		for group := range clients.groups {
			err = errors.Join(err, clients.sendGroup(group, op.message))
		}
		return err
	}
	return nil
}

func envelope(payload []byte) ([]byte, error) {
	if !json.Valid(payload) {
		return nil, errors.New("ws: message must contain valid json")
	}
	var out bytes.Buffer
	out.Grow(len(payload) + 20)
	out.WriteString(`{"code":200,"data":`)
	out.Write(payload)
	out.WriteByte('}')
	return out.Bytes(), nil
}

func (s *connectionSet) add(c *client) {
	if old := s.groups[c.group][c.id]; old != nil {
		s.remove(old)
	}
	if s.groups[c.group] == nil {
		s.groups[c.group] = make(map[string]*client)
	}
	s.groups[c.group][c.id] = c
	s.clients++
	s.changed = true
}

func (s *connectionSet) remove(c *client) {
	group := s.groups[c.group]
	if group[c.id] != c {
		return
	}
	delete(group, c.id)
	s.clients--
	s.changed = true
	if len(group) == 0 {
		delete(s.groups, c.group)
	}
	_ = c.conn.Close()
	// The command loop is the only sender and closer of outgoing.
	close(c.outgoing)
}

func (s *connectionSet) send(c *client, message []byte) error {
	select {
	case c.outgoing <- message:
		return nil
	default:
		s.remove(c)
		return ErrSlowClient
	}
}

func (s *connectionSet) sendGroup(group string, message []byte) error {
	var err error
	for _, c := range s.groups[group] {
		err = errors.Join(err, s.send(c, message))
	}
	return err
}

func (s *connectionSet) publishStats(stats *atomic.Pointer[counts]) {
	if !s.changed {
		return
	}
	stats.Store(&counts{groups: len(s.groups), clients: s.clients})
	s.changed = false
}
