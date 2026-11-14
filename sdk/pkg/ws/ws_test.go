package ws

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialedConnection(t *testing.T) (*websocket.Conn, <-chan []byte) {
	t.Helper()
	received := make(chan []byte, 128)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			select {
			case received <- msg:
			default:
			}
		}
	}))
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, received
}

func runningManager(t *testing.T, opts ...Option) *Manager {
	t.Helper()
	m, err := New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = m.Run(t.Context()) }()
	deadline := time.After(time.Second)
	for {
		m.mu.Lock()
		ready := m.isRunning
		m.mu.Unlock()
		if ready {
			break
		}
		select {
		case <-deadline:
			t.Fatal("manager did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return m
}

func TestShutdownEndsConnectionWorkers(t *testing.T) {
	m := runningManager(t)
	conn, received := dialedConnection(t)
	if err := m.Connect(t.Context(), "test", "group", conn); err != nil {
		t.Fatal(err)
	}
	if err := m.Send(t.Context(), "test", "group", []byte(`{"event":"ready"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-received:
		if string(msg) != `{"code":200,"data":{"event":"ready"}}` {
			t.Fatalf("message=%s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("message was not delivered")
	}
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.Info()["clientLen"] != 0 {
		t.Fatal("clients survived shutdown")
	}
	if err := m.SendAll(t.Context(), []byte(`null`)); !errors.Is(err, ErrClosed) {
		t.Fatalf("send after close: %v", err)
	}
}

func TestRegistrationAndBroadcastHaveOneQueueOwner(t *testing.T) {
	m := runningManager(t)
	var wg sync.WaitGroup
	for i := range 8 {
		conn, _ := dialedConnection(t)
		wg.Go(func() {
			id := strconv.Itoa(i)
			if err := m.Connect(t.Context(), id, "group", conn); err != nil {
				t.Error(err)
				return
			}
			for range 30 {
				if err := m.SendAll(t.Context(), []byte(`"event"`)); err != nil && !errors.Is(err, ErrSlowClient) {
					t.Error(err)
					return
				}
			}
			if err := m.Disconnect(t.Context(), id, "group"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkOptionsRejectInvalidBudgets(t *testing.T) {
	for name, option := range map[string]Option{
		"negative message limit": WithMaxMessageBytes(-1),
		"negative write timeout": WithWriteTimeout(-time.Second),
		"nil option":             nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(option); err == nil {
				t.Fatal("invalid option accepted")
			}
		})
	}
	m, err := New(WithMaxMessageBytes(0), WithWriteTimeout(0))
	if err != nil {
		t.Fatal(err)
	}
	if m.opts.maxMessageBytes != 1<<20 || m.opts.writeTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", m.opts)
	}
}

func managedPeer(t *testing.T, m *Manager) *websocket.Conn {
	t.Helper()
	accepted := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			accepted <- conn
		}
	}))
	t.Cleanup(srv.Close)
	dialer := websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		conn, err := d.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			if err := tcp.SetReadBuffer(1024); err != nil {
				_ = conn.Close()
				return nil, err
			}
		}
		return conn, nil
	}}
	peer, _, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	conn := <-accepted
	if err := m.Connect(t.Context(), "peer", "group", conn); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return peer
}

func waitForDisconnect(t *testing.T, m *Manager) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for m.Info()["clientLen"] != 0 {
		select {
		case <-deadline.C:
			t.Fatal("connection worker did not release its client")
		case <-tick.C:
		}
	}
}

func TestIncomingMessageLimitDisconnectsPeer(t *testing.T) {
	m := runningManager(t, WithMaxMessageBytes(16))
	peer := managedPeer(t, m)
	if err := peer.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", 17))); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := peer.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("oversized message close = %v", err)
	}
	waitForDisconnect(t, m)
}

func TestWriteBudgetDisconnectsStalledPeer(t *testing.T) {
	m := runningManager(t, WithWriteTimeout(20*time.Millisecond))
	_ = managedPeer(t, m)
	payload := []byte(`"` + strings.Repeat("x", 4<<20) + `"`)
	if err := m.Send(t.Context(), "peer", "group", payload); err != nil {
		t.Fatal(err)
	}
	waitForDisconnect(t, m)
}

func TestStatsTrackReplacementAndDisconnection(t *testing.T) {
	m := runningManager(t)
	assertStats := func(groups, clients int) {
		t.Helper()
		stats := m.Info()
		if stats["groupLen"] != groups || stats["clientLen"] != clients {
			t.Fatalf("membership stats = %v, want %d groups and %d clients", stats, groups, clients)
		}
	}
	first, _ := dialedConnection(t)
	second, _ := dialedConnection(t)
	replacement, received := dialedConnection(t)
	if err := m.Connect(t.Context(), "first", "group", first); err != nil {
		t.Fatal(err)
	}
	if err := m.Connect(t.Context(), "second", "group", second); err != nil {
		t.Fatal(err)
	}
	assertStats(1, 2)
	if err := m.Connect(t.Context(), "first", "group", replacement); err != nil {
		t.Fatal(err)
	}
	assertStats(1, 2)
	if err := m.Send(t.Context(), "first", "group", []byte(`null`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("replacement did not receive its message")
	}
	assertStats(1, 2)
	if err := m.Disconnect(t.Context(), "second", "group"); err != nil {
		t.Fatal(err)
	}
	assertStats(1, 1)
	if err := m.Disconnect(t.Context(), "first", "group"); err != nil {
		t.Fatal(err)
	}
	assertStats(0, 0)
}

func TestSlowClientRemovalUpdatesMembershipStats(t *testing.T) {
	m := runningManager(t, WithWriteTimeout(10*time.Second))
	_ = managedPeer(t, m)
	payload := []byte(strings.Repeat("x", 4<<20))
	for range 2048 {
		err := m.submit(t.Context(), command{
			kind: commandSend, id: "peer", group: "group", message: payload,
		})
		if errors.Is(err, ErrSlowClient) {
			stats := m.Info()
			if stats["groupLen"] != 0 || stats["clientLen"] != 0 {
				t.Fatalf("slow client remained counted: %v", stats)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("stalled connection never filled its send queue")
}
