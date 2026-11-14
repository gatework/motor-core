package queue

import (
	"errors"
	"testing"

	"github.com/gatework/motor-core/storage"
)

func TestRunAfterShutdownReturns(t *testing.T) {
	q := New(1)
	if err := q.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := q.Run(t.Context()); !errors.Is(err, storage.ErrQueueClosed) {
		t.Fatalf("Run after Shutdown returned %v", err)
	}
}
