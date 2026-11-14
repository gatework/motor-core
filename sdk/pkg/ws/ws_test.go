package ws

import (
	"context"
	"errors"
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

func runningManager(t *testing.T) *Manager {
	t.Helper()
	m := New()
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
		wg.Add(1)
		go func() {
			defer wg.Done()
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
		}()
	}
	wg.Wait()
	if err := m.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}
