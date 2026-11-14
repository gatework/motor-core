// Package queue provides bounded in-process message delivery.
package queue

import (
	"context"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gatework/motor-core/storage"
)

const (
	defaultBuffer = 1024
	maxAttempts   = 4
)

type subscription struct {
	handler  storage.Handler
	messages chan storage.Message
}

// Memory is a bounded in-process queue. Each subscribed topic has its own
// buffer and worker, so a slow handler does not block unrelated topics.
// Messages are not durable and do not survive a process restart.
type Memory struct {
	mu         sync.Mutex
	topics     map[string]*subscription
	buffer     int
	isStarted  bool
	isClosed   bool
	cancel     context.CancelFunc
	stop       chan struct{}
	done       chan struct{}
	publishers sync.WaitGroup
	workers    sync.WaitGroup
	seq        atomic.Int64
}

var _ storage.Queue = (*Memory)(nil)

// New constructs a queue with size buffered messages per topic. A non-positive
// size uses the default. Subscribers must be registered before Run.
func New(size int) *Memory {
	if size <= 0 {
		size = defaultBuffer
	}
	return &Memory{
		topics: make(map[string]*subscription),
		buffer: size,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (*Memory) String() string { return "memory" }

// Subscribe registers a topic handler before consumption begins.
func (q *Memory) Subscribe(ctx context.Context, topic string, h storage.Handler) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if h == nil {
		return storage.ErrNilHandler
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.isClosed {
		return storage.ErrQueueClosed
	}
	if q.isStarted {
		return storage.ErrQueueAlreadyStarted
	}
	if _, exists := q.topics[topic]; exists {
		return storage.ErrTopicAlreadySubscribed
	}
	q.topics[topic] = &subscription{
		handler:  h,
		messages: make(chan storage.Message, q.buffer),
	}
	return nil
}

// Publish copies the payload map and enqueues it within the caller budget.
func (q *Memory) Publish(ctx context.Context, msg storage.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	q.mu.Lock()
	if q.isClosed {
		q.mu.Unlock()
		return storage.ErrQueueClosed
	}
	sub := q.topics[msg.Topic]
	if sub == nil {
		q.mu.Unlock()
		return storage.ErrNoHandler
	}
	// Shutdown publishes its stop signal under this lock and waits for every
	// admitted producer before closing any buffer. A producer blocked on a
	// full buffer therefore unblocks without sending to a closed channel.
	q.publishers.Add(1)
	q.mu.Unlock()
	defer q.publishers.Done()

	msg.ID = strconv.FormatInt(q.seq.Add(1), 10)
	msg.Attempts = 1
	msg.Values = maps.Clone(msg.Values)
	select {
	case sub.messages <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-q.stop:
		return storage.ErrQueueClosed
	}
}

// Run consumes subscribed topics until cancellation or shutdown completes.
func (q *Memory) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	q.mu.Lock()
	if q.isClosed {
		q.mu.Unlock()
		return storage.ErrQueueClosed
	}
	if q.isStarted {
		q.mu.Unlock()
		return storage.ErrQueueAlreadyStarted
	}
	q.startLocked(ctx)
	q.mu.Unlock()

	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		q.stopWorkers(ctx)
		<-q.done
		return nil
	}
}

// Shutdown rejects new publishers and drains accepted messages within ctx.
func (q *Memory) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cancel := q.stopWorkers(ctx)
	stopCancel := context.AfterFunc(ctx, cancel)
	defer stopCancel()
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

// startLocked starts the owned workers once. Shutdown uses it as well, so a
// message accepted before Run is still delivered when the queue is shut down.
func (q *Memory) startLocked(ctx context.Context) {
	workCtx, cancel := context.WithCancel(ctx)
	q.cancel = cancel
	q.isStarted = true
	q.workers.Add(len(q.topics))
	for _, sub := range q.topics {
		go func() {
			defer q.workers.Done()
			for msg := range sub.messages {
				q.deliver(workCtx, sub.handler, msg)
			}
		}()
	}
}

func (q *Memory) deliver(ctx context.Context, h storage.Handler, msg storage.Message) {
	for {
		if msg.Attempts > 1 {
			if ctx.Err() != nil {
				return
			}
			q.mu.Lock()
			closed := q.isClosed
			q.mu.Unlock()
			if closed {
				return
			}
		}
		if err := h(ctx, msg); err == nil || msg.Attempts >= maxAttempts {
			return
		}
		// Once shutdown starts, buffered messages still get one delivery, but
		// retries must not hold up the drain or reinsert into a full buffer.
		timer := time.NewTimer(time.Duration(msg.Attempts) * time.Second)
		select {
		case <-timer.C:
			msg.Attempts++
		case <-q.stop:
			timer.Stop()
			return
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

// stopWorkers establishes the acceptance barrier before closing the buffers.
// The coordinator is created once and retained through done, so callers with
// different shutdown budgets wait for the same completion.
func (q *Memory) stopWorkers(ctx context.Context) context.CancelFunc {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.isClosed {
		q.isClosed = true
		close(q.stop)
		if !q.isStarted {
			q.startLocked(context.WithoutCancel(ctx))
		}
		go func() {
			q.publishers.Wait()
			for _, sub := range q.topics {
				close(sub.messages)
			}
			q.workers.Wait()
			q.cancel()
			close(q.done)
		}()
	}
	return q.cancel
}
