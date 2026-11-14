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
	ErrClosed     = errors.New("ws: manager closed")
	ErrNotRunning = errors.New("ws: manager not running")
	ErrNoClient   = errors.New("ws: client not found")
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
	groups := make(map[string]map[string]*client)
	clientCount := 0
	statsChanged := false
	closeClient := func(c *client) {
		g := groups[c.group]
		if g[c.id] != c {
			return
		}
		delete(g, c.id)
		clientCount--
		statsChanged = true
		if len(g) == 0 {
			delete(groups, c.group)
		}
		_ = c.conn.Close()
		// Only this loop sends to or closes outgoing.
		close(c.outgoing)
	}
	publishStats := func() {
		if statsChanged {
			m.stats.Store(&counts{groups: len(groups), clients: clientCount})
			statsChanged = false
		}
	}
	defer func() {
		m.mu.Lock()
		if !m.isClosed {
			m.isClosed = true
			close(m.stop)
		}
		m.mu.Unlock()
		for _, g := range groups {
			for _, c := range g {
				closeClient(c)
			}
		}
		publishStats()
		m.workers.Wait()
		close(m.done)
	}()
	send := func(c *client, msg []byte) error {
		select {
		case c.outgoing <- msg:
			return nil
		default:
			closeClient(c)
			return ErrSlowClient
		}
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-m.stop:
			return nil
		case op := <-m.commands:
			var err error
			switch op.kind {
			case commandConnect:
				c := op.client
				if old := groups[c.group][c.id]; old != nil {
					closeClient(old)
				}
				if groups[c.group] == nil {
					groups[c.group] = make(map[string]*client)
				}
				groups[c.group][c.id] = c
				clientCount++
				statsChanged = true
				c.conn.SetReadLimit(m.opts.maxMessageBytes)
				m.workers.Add(2)
				go func() { defer m.workers.Done(); m.read(ctx, c) }()
				go func() { defer m.workers.Done(); m.write(ctx, c) }()
			case commandDisconnect:
				c := groups[op.group][op.id]
				if c != nil && (op.client == nil || op.client == c) {
					closeClient(c)
				}
			case commandSend:
				c := groups[op.group][op.id]
				if c == nil || op.client != nil && op.client != c {
					err = ErrNoClient
				} else {
					err = send(c, op.message)
				}
			case commandGroup:
				for _, c := range groups[op.group] {
					err = errors.Join(err, send(c, op.message))
				}
			case commandBroadcast:
				for _, g := range groups {
					for _, c := range g {
						err = errors.Join(err, send(c, op.message))
					}
				}
			}
			publishStats()
			if op.reply != nil {
				op.reply <- err
			}
		}
	}
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

// Connect transfers ownership of conn only when registration succeeds.
func (m *Manager) Connect(ctx context.Context, id, group string, conn *websocket.Conn) error {
	if conn == nil || id == "" {
		return errors.New("ws: connection and id are required")
	}
	return m.submit(ctx, command{kind: commandConnect, id: id, group: group, client: &client{id: id, group: group, conn: conn, outgoing: make(chan []byte, 1024)}})
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
func (m *Manager) Send(ctx context.Context, id, group string, payload []byte) error {
	data, err := envelope(payload)
	if err != nil {
		return err
	}
	return m.submit(ctx, command{kind: commandSend, id: id, group: group, message: data})
}
func (m *Manager) SendGroup(ctx context.Context, group string, payload []byte) error {
	data, err := envelope(payload)
	if err != nil {
		return err
	}
	return m.submit(ctx, command{kind: commandGroup, group: group, message: data})
}
func (m *Manager) SendAll(ctx context.Context, payload []byte) error {
	data, err := envelope(payload)
	if err != nil {
		return err
	}
	return m.submit(ctx, command{kind: commandBroadcast, message: data})
}
func (m *Manager) Disconnect(ctx context.Context, id, group string) error {
	return m.submit(ctx, command{kind: commandDisconnect, id: id, group: group})
}

func (m *Manager) Info() map[string]any {
	m.init()
	v := m.stats.Load()
	return map[string]any{"groupLen": v.groups, "clientLen": v.clients, "chanRegisterLen": 0, "chanUnregisterLen": 0, "chanMessageLen": len(m.commands), "chanGroupMessageLen": 0, "chanBroadCastMessageLen": 0}
}
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
