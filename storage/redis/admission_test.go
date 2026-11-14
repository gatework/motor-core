package redis

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage"
	goredis "github.com/redis/go-redis/v9"
)

type blockedPublishClient struct {
	goredis.UniversalClient
	entered chan struct{}
	release chan struct{}
	closed  atomic.Int64
}

func (c *blockedPublishClient) XGroupCreateMkStream(ctx context.Context, _, _, _ string) *goredis.StatusCmd {
	return goredis.NewStatusResult("OK", nil)
}

func (c *blockedPublishClient) XAdd(ctx context.Context, _ *goredis.XAddArgs) *goredis.StringCmd {
	close(c.entered)
	<-c.release
	return goredis.NewStringResult("1-0", nil)
}

func (c *blockedPublishClient) Close() error {
	c.closed.Add(1)
	return nil
}

func TestShutdownWaitsForAdmittedPublish(t *testing.T) {
	client := &blockedPublishClient{entered: make(chan struct{}), release: make(chan struct{})}
	q := newQueue(client, true, QueueOptions{})
	if err := q.Subscribe(t.Context(), "orders", func(context.Context, storage.Message) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var unblock sync.Once
	release := func() { unblock.Do(func() { close(client.release) }) }
	defer release()
	published := make(chan error, 1)
	go func() { published <- q.Publish(t.Context(), storage.Message{Topic: "orders"}) }()
	<-client.entered
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := q.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown with an admitted blocked publish = %v, want deadline exceeded", err)
	}
	if client.closed.Load() != 0 {
		t.Fatal("owned client closed before the admitted publish completed")
	}
	if err := q.Publish(t.Context(), storage.Message{Topic: "orders"}); !errors.Is(err, storage.ErrQueueClosed) {
		t.Fatalf("late Publish = %v, want queue closed", err)
	}
	release()
	if err := <-published; err != nil {
		t.Fatalf("admitted Publish = %v", err)
	}
	if err := q.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if client.closed.Load() != 1 {
		t.Fatalf("owned client closed %d times, want 1", client.closed.Load())
	}
}
