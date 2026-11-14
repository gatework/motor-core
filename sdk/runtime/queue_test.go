package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/queue"
)

func TestQueueForwardsSubscriptionAndDelivery(t *testing.T) {
	q := NewQueue("tenant", queue.New(10))
	got := make(chan storage.Message, 1)
	if err := q.Subscribe(t.Context(), "orders", func(_ context.Context, msg storage.Message) error {
		got <- msg
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Shutdown(context.Background()) }()
	go func() { _ = q.Run(t.Context()) }()
	if err := q.Publish(t.Context(), storage.Message{Topic: "orders", Values: map[string]any{"id": "1"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-got:
		if msg.Values[storage.PrefixKey] != "tenant" || msg.Values["id"] != "1" {
			t.Fatalf("forwarding changed payload: %#v", msg.Values)
		}
	case <-time.After(time.Second):
		t.Fatal("the subscribed consumer received no message")
	}
}
