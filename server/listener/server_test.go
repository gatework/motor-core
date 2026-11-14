package listener

import (
	"context"
	"net"
	"net/http"
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
