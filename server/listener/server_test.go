package listener

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestCancellationStopsListener(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	ready := make(chan struct{})
	s := New("test", WithAddr("127.0.0.1:0"), WithStartedHook(func() { close(ready) }), WithEndHook(func() { close(stopped) }))
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
	s := New("test", WithAddr(l.Addr().String()), WithHandler(http.NotFoundHandler()))
	if err := s.Run(t.Context()); err == nil {
		t.Fatal("occupied port accepted")
	}
	badTLS := New("tls", WithAddr("127.0.0.1:0"), WithCert("missing.pem"), WithKey("missing.key"))
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
	s := New(
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
