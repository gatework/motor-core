package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/queue"
)

// Separate tenant views must reach one application-owned fallback queue.
func TestQueueForTenantSharesFallback(t *testing.T) {
	app := New()
	got := make(chan storage.Message, 1)
	consumer := app.QueueForTenant("consumer")
	if err := consumer.Subscribe(t.Context(), "orders", func(_ context.Context, msg storage.Message) error {
		got <- msg
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	go func() { _ = consumer.Run(t.Context()) }()
	defer func() { _ = app.Queue().Shutdown(context.Background()) }()
	if err := app.QueueForTenant("producer").Publish(t.Context(), storage.Message{Topic: "orders"}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if msg.Values[storage.PrefixKey] != "producer" {
			t.Fatalf("producer tenant was lost: %#v", msg.Values)
		}
	case <-time.After(time.Second):
		t.Fatal("producer and consumer reached different fallback queues")
	}
}

func TestConfiguredQueueAndFallbackAreUsed(t *testing.T) {
	app := New()
	fallback := app.Queue()
	configured := queue.New(8)
	defer func() { _ = fallback.Shutdown(context.Background()); _ = configured.Shutdown(context.Background()) }()
	app.SetQueue(configured)
	if app.Queue() != configured || app.QueueForTenant("tenant").queue != configured {
		t.Fatal("queue accessors ignored the configured queue")
	}
	app.SetQueue(nil)
	if app.Queue() != fallback {
		t.Fatal("clearing configuration replaced the shared fallback queue")
	}
}

func TestStorageSwapIsRaceFree(t *testing.T) {
	app := New()
	const rounds = 2000
	var wg sync.WaitGroup
	wg.Go(func() {
		for range rounds {
			app.SetQueue(queue.New(8))
			app.SetCache(nil)
		}
	})
	wg.Go(func() {
		for range rounds {
			_ = app.QueueForTenant("tenant")
			_ = app.Cache()
		}
	})
	wg.Wait()
}
