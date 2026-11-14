package runtime

import (
	"context"
	"testing"

	"github.com/gatework/motor-core/storage"
)

type recordingQueue struct {
	storage.Queue
	message storage.Message
}

func (q *recordingQueue) Publish(_ context.Context, msg storage.Message) error {
	q.message = msg
	return nil
}

func TestPublishTagsTenantWithoutMutatingInput(t *testing.T) {
	for _, values := range []map[string]any{nil, {"id": "7", storage.PrefixKey: "original"}} {
		backing := &recordingQueue{}
		q := NewQueue("tenant", backing)
		msg := storage.Message{Topic: "orders", Values: values}
		if err := q.Publish(t.Context(), msg); err != nil {
			t.Fatal(err)
		}
		if got := backing.message.Values[storage.PrefixKey]; got != "tenant" {
			t.Fatalf("published tenant = %v", got)
		}
		if values == nil {
			if msg.Values != nil {
				t.Fatal("Publish changed the caller's nil values")
			}
			continue
		}
		if values["id"] != "7" || values[storage.PrefixKey] != "original" {
			t.Fatalf("Publish mutated its input: %#v", values)
		}
		if got := backing.message.Values["id"]; got != "7" {
			t.Fatalf("Publish lost an existing value: %v", got)
		}
	}
}
