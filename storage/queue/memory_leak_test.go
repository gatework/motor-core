package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gatework/motor-core/storage"
)

func TestPublishToFullBufferHonoursCancellation(t *testing.T) {
	q := New(1)
	if err := q.Subscribe(t.Context(), "orders", func(context.Context, storage.Message) error { return nil }); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Shutdown(context.Background()) }()
	if err := q.Publish(t.Context(), storage.Message{Topic: "orders"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := q.Publish(ctx, storage.Message{Topic: "orders"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Publish to a full buffer returned %v", err)
	}
}
