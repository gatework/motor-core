package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testRunnable struct {
	name  string
	start func(context.Context) error
}

func (r testRunnable) String() string                { return r.name }
func (r testRunnable) Attempt() bool                 { return true }
func (r testRunnable) Run(ctx context.Context) error { return r.start(ctx) }

func TestStartCancellation(t *testing.T) {
	for _, withService := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "service"}[withService], func(t *testing.T) {
			s := New()
			if withService {
				s.Add("wait", testRunnable{name: "wait", start: func(ctx context.Context) error { <-ctx.Done(); return nil }})
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- s.Run(ctx) }()
			cancel()
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("Start did not return after cancellation")
			}
		})
	}
}
func TestStartFailureCancelsPeers(t *testing.T) {
	failure := errors.New("start failed")
	stopped := make(chan struct{})
	s := New()
	s.Add("fail", testRunnable{name: "fail", start: func(context.Context) error { return failure }})
	s.Add("peer", testRunnable{name: "peer", start: func(ctx context.Context) error { <-ctx.Done(); close(stopped); return nil }})
	done := make(chan error, 1)
	go func() { done <- s.Run(t.Context()) }()
	select {
	case err := <-done:
		if !errors.Is(err, failure) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not propagate failure")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("peer was not joined")
	}
}
