package runtime

import (
	"context"
	"github.com/gatework/motor-core/storage"
	"maps"
)

// Queue is a tenant-tagging view. The backing queue's owner controls its lifetime.
type Queue struct {
	prefix string
	queue  storage.Queue
}

func NewQueue(prefix string, q storage.Queue) *Queue { return &Queue{prefix: prefix, queue: q} }
func (q *Queue) Publish(ctx context.Context, msg storage.Message) error {
	msg.Values = maps.Clone(msg.Values)
	if msg.Values == nil {
		msg.Values = make(map[string]any)
	}
	msg.Values[storage.PrefixKey] = q.prefix
	return q.queue.Publish(ctx, msg)
}
func (q *Queue) Subscribe(ctx context.Context, topic string, h storage.Handler) error {
	return q.queue.Subscribe(ctx, topic, h)
}
func (q *Queue) Run(ctx context.Context) error      { return q.queue.Run(ctx) }
func (q *Queue) Shutdown(ctx context.Context) error { return q.queue.Shutdown(ctx) }
