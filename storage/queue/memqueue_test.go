package queue_test

import (
	"testing"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor-core/storage/cachetest"
	"github.com/gatework/motor-core/storage/queue"
)

func TestMemQueueConformance(t *testing.T) {
	cachetest.RunQueue(t, func(t *testing.T) storage.Queue {
		t.Helper()
		return queue.New(64)
	})
}
