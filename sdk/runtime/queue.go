package runtime

import (
	"context"
	"maps"

	"github.com/gatework/motor-core/storage"
)

// Queue is a tenant-tagging view. The backing queue's owner controls its lifetime.
type Queue struct {
	prefix string
	queue  storage.Queue
}

// NewQueue constructs a tenant-tagging view without creating a worker.
func NewQueue(prefix string, q storage.Queue) *Queue { return &Queue{prefix: prefix, queue: q} }

// Publish tags a message with the tenant before publishing a copy.
func (q *Queue) Publish(ctx context.Context, msg storage.Message) error {
	msg.Values = maps.Clone(msg.Values)
	if msg.Values == nil {
		msg.Values = make(map[string]any)
	}
	msg.Values[storage.PrefixKey] = q.prefix
	return q.queue.Publish(ctx, msg)
}

// Subscribe registers a topic handler on the backing queue.
func (q *Queue) Subscribe(ctx context.Context, topic string, h storage.Handler) error {
	return q.queue.Subscribe(ctx, topic, h)
}

// Run starts the backing queue and waits for cancellation or failure.
func (q *Queue) Run(ctx context.Context) error { return q.queue.Run(ctx) }

// Shutdown stops admissions and waits for owned connection workers.
func (q *Queue) Shutdown(ctx context.Context) error { return q.queue.Shutdown(ctx) }
