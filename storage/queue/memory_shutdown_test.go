package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gatework/motor-core/storage"
)

func TestShutdownWaitsForAdmittedPublishers(t *testing.T) {
	q := New(1)
	var delivered atomic.Int64
	if err := q.Subscribe(t.Context(), "orders", func(context.Context, storage.Message) error {
		delivered.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.Publish(t.Context(), storage.Message{Topic: "orders"}); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int64
	accepted.Store(1)
	var producers sync.WaitGroup
	for range 32 {
		producers.Go(func() {
			err := q.Publish(t.Context(), storage.Message{Topic: "orders"})
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, storage.ErrQueueClosed) {
				t.Error(err)
			}
		})
	}
	if err := q.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	producers.Wait()
	if got, want := delivered.Load(), accepted.Load(); got != want {
		t.Fatalf("shutdown delivered %d of %d accepted messages", got, want)
	}
}

func TestShutdownBudgetAndRepeatedWait(t *testing.T) {
	q := New(1)
	entered := make(chan struct{})
	release := make(chan struct{})
	if err := q.Subscribe(t.Context(), "orders", func(context.Context, storage.Message) error {
		close(entered)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runCtx, stopRun := context.WithCancel(t.Context())
	defer stopRun()
	runDone := make(chan error, 1)
	go func() { runDone <- q.Run(runCtx) }()
	if err := q.Publish(t.Context(), storage.Message{Topic: "orders"}); err != nil {
		t.Fatal(err)
	}
	<-entered
	stopRun()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := q.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("shutdown with an uncooperative handler: %v", err)
	}
	select {
	case err := <-runDone:
		t.Errorf("Run returned before its handler exited: %v", err)
	default:
	}
	close(release)
	if err := q.Shutdown(t.Context()); err != nil {
		t.Fatalf("a second caller could not wait for completion: %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("Run after its worker finished: %v", err)
	}
}

func TestRetryLimitAndShutdownBackoff(t *testing.T) {
	for name, stopRetry := range map[string]bool{"four total deliveries": false, "shutdown cancels backoff": true} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				q := New(1)
				var attempts []int
				if err := q.Subscribe(t.Context(), "orders", func(_ context.Context, msg storage.Message) error {
					attempts = append(attempts, msg.Attempts)
					return errors.New("retry")
				}); err != nil {
					t.Fatal(err)
				}
				go func() { _ = q.Run(t.Context()) }()
				if err := q.Publish(t.Context(), storage.Message{Topic: "orders", Attempts: 99}); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				want := 1
				if !stopRetry {
					time.Sleep(7 * time.Second)
					want = 4
				}
				if err := q.Shutdown(t.Context()); err != nil {
					t.Fatal(err)
				}
				if len(attempts) != want {
					t.Fatalf("attempts %v, want %d deliveries", attempts, want)
				}
				for i, attempt := range attempts {
					if attempt != i+1 {
						t.Fatalf("attempts are not sequential: %v", attempts)
					}
				}
			})
		})
	}
}
