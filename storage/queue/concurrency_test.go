// Package queue_test verifies queue isolation and concurrent access.
package queue

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage"
)

func TestConcurrentPublishersShareOneQueue(t *testing.T) {
	q := New(4096)
	var received atomic.Int64
	if err := q.Subscribe(t.Context(), "orders", func(context.Context, storage.Message) error {
		received.Add(1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	go func() { _ = q.Run(t.Context()) }()

	const producers, each = 32, 50
	var wg sync.WaitGroup
	for range producers {
		wg.Go(func() {
			for range each {
				if err := q.Publish(t.Context(), storage.Message{Topic: "orders"}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if err := q.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := received.Load(); got != producers*each {
		t.Fatalf("delivered %d messages, want %d", got, producers*each)
	}
}

func TestSlowTopicDoesNotBlockAnotherTopic(t *testing.T) {
	q := New(8)
	blocked := make(chan struct{})
	release := make(chan struct{})
	got := make(chan struct{})
	if err := q.Subscribe(t.Context(), "slow", func(context.Context, storage.Message) error {
		close(blocked)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.Subscribe(t.Context(), "fast", func(context.Context, storage.Message) error {
		close(got)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	go func() { _ = q.Run(t.Context()) }()
	defer func() { close(release); _ = q.Shutdown(context.Background()) }()
	if err := q.Publish(t.Context(), storage.Message{Topic: "slow"}); err != nil {
		t.Fatal(err)
	}
	<-blocked
	if err := q.Publish(t.Context(), storage.Message{Topic: "fast"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("a blocked topic held up an unrelated handler")
	}
}

func benchPublish(b *testing.B, size, topics int) {
	b.Helper()
	q := New(size)
	for i := range topics {
		if err := q.Subscribe(b.Context(), "s"+strconv.Itoa(i), func(context.Context, storage.Message) error {
			return nil
		}); err != nil {
			b.Fatal(err)
		}
	}
	go func() { _ = q.Run(b.Context()) }()
	b.Cleanup(func() { _ = q.Shutdown(context.Background()) })
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			if err := q.Publish(b.Context(), storage.Message{Topic: "s" + strconv.Itoa(i%topics)}); err != nil {
				b.Error(err)
			}
			i++
		}
	})
}

func BenchmarkMemoryQueuePublish(b *testing.B)           { benchPublish(b, 100, 1) }
func BenchmarkMemoryQueuePublishLargePool(b *testing.B)  { benchPublish(b, 10000, 1) }
func BenchmarkMemoryQueuePublishMultiTopic(b *testing.B) { benchPublish(b, 10000, 8) }
