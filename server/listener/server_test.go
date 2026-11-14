package listener

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func mustServer(t *testing.T, name string, opts ...Option) *Server {
	t.Helper()
	s, err := New(name, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCancellationStopsListener(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	ready := make(chan struct{})
	s := mustServer(t, "test", WithAddr("127.0.0.1:0"), WithStartedHook(func() { close(ready) }), WithEndHook(func() { close(stopped) }))
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("listener did not start")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	})
	cancel()
	select {
	case <-stopped:
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not shut down HTTP server")
	}
}
func TestStartReportsBindFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := mustServer(t, "test", WithAddr(l.Addr().String()), WithHandler(http.NotFoundHandler()))
	if err := s.Run(t.Context()); err == nil {
		t.Fatal("occupied port accepted")
	}
	badTLS := mustServer(t, "tls", WithAddr("127.0.0.1:0"), WithCert("missing.pem"), WithKey("missing.key"))
	if err := badTLS.Run(t.Context()); err == nil {
		t.Fatal("invalid tls configuration accepted")
	}
}

func TestShutdownTimeoutRetainsDrainUntilRequestsFinish(t *testing.T) {
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := port.Addr().String()
	_ = port.Close()
	ready := make(chan struct{})
	entered := make(chan struct{})
	release := make(chan struct{})
	ended := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	s := mustServer(t,
		"drain",
		WithAddr(addr),
		WithStartedHook(func() { close(ready) }),
		WithEndHook(func() { close(ended) }),
		WithHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusOK)
		})),
	)
	runDone := make(chan error, 1)
	go func() { runDone <- s.Run(ctx) }()
	select {
	case <-ready:
	case err := <-runDone:
		t.Fatalf("listener did not start: %v", err)
	case <-time.After(time.Second):
		t.Fatal("listener did not start")
	}
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + addr)
		if err == nil {
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	budget, stop := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer stop()
	if err := s.Shutdown(budget); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- s.Shutdown(t.Context()) }()
	select {
	case err := <-secondDone:
		t.Fatalf("second shutdown completed while handler was active: %v", err)
	case <-ended:
		t.Fatal("end hook ran while handler was active")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if err := <-runDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	default:
		t.Fatal("end hook did not run after drain")
	}
}

func TestNetworkOptionsRejectInvalidBudgets(t *testing.T) {
	for name, option := range map[string]Option{
		"negative header timeout": WithReadHeaderTimeout(-time.Second),
		"nil handler":             WithHandler(nil),
		"empty address":           WithAddr(""),
		"missing key":             WithCert("certificate.pem"),
		"nil option":              nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New("test", option); err == nil {
				t.Fatal("invalid option accepted")
			}
		})
	}
	s := mustServer(t, "defaults", WithReadHeaderTimeout(0), WithIdleTimeout(0))
	if s.opts.readHeaderTimeout != 10*time.Second || s.opts.idleTimeout != time.Minute {
		t.Fatalf("unexpected network defaults: %+v", s.opts)
	}
	if err := s.Options(WithAddr("127.0.0.1:0"), WithIdleTimeout(-time.Second)); err == nil {
		t.Fatal("invalid option update accepted")
	}
	if s.opts.addr != ":8080" {
		t.Fatal("failed option update was partially applied")
	}
}

func TestHeaderBudgetDoesNotBoundStreamingResponses(t *testing.T) {
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := port.Addr().String()
	_ = port.Close()
	ready := make(chan struct{})
	s := mustServer(
		t,
		"budgets",
		WithAddr(addr),
		WithReadHeaderTimeout(20*time.Millisecond),
		WithStartedHook(func() { close(ready) }),
		WithHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "first")
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
			_, _ = io.WriteString(w, "second")
		})),
	)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("listener did not start")
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: test\r\nX-Slow: "); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(conn); err != nil {
		t.Fatalf("incomplete headers were not closed within the budget: %v", err)
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "firstsecond" {
		t.Fatalf("stream ended early: %q", body)
	}
}
