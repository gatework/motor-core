package logger

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// AsyncConfig 异步日志配置
type AsyncConfig struct {
	// BufferSize 缓冲区大小（队列长度）
	BufferSize int
	// FlushInterval 刷新间隔（定时批量写入）
	FlushInterval time.Duration
	// OnDropped 队列满时回调（用于监控丢弃的日志）
	OnDropped func(level Level, msg string)
	// DropPolicy 队列满时策略
	// - "drop": 丢弃新日志（默认）
	// - "block": 阻塞等待
	// - "sample": 降级采样（每10条记录1条）
	DropPolicy string
}

// DefaultAsyncConfig 默认异步配置（生产环境推荐）
var DefaultAsyncConfig = AsyncConfig{
	BufferSize:    10000,                  // ten thousand entries
	FlushInterval: 100 * time.Millisecond, // 每100ms刷新
	DropPolicy:    "drop",                 // drop when the queue is full
	OnDropped:     nil,                    // no callback on a drop
}

// logEntry owns its field slice. Values stored in Field.Any remain borrowed.
type logEntry struct {
	ctx    context.Context
	level  Level
	msg    string
	fields []Field
	synced chan struct{}
}

// Async owns one bounded worker and the wrapped logger. Close stops admissions,
// drains every accepted entry, and closes the wrapped root. Derived loggers
// carry fields only and cannot create or close another worker.
type Async struct {
	logger       Logger
	config       AsyncConfig
	buffer       chan logEntry
	stop         chan struct{}
	done         chan struct{}
	mu           sync.RWMutex
	closed       bool
	once         sync.Once
	closeErr     error
	droppedCount atomic.Uint64
	fullCount    atomic.Uint64
}

func NewAsyncLogger(log Logger, config AsyncConfig) *Async {
	if log == nil {
		log = Default()
	}
	if config.BufferSize <= 0 {
		config.BufferSize = DefaultAsyncConfig.BufferSize
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = DefaultAsyncConfig.FlushInterval
	}
	if config.DropPolicy == "" {
		config.DropPolicy = "drop"
	}
	async := &Async{
		logger: log, config: config, buffer: make(chan logEntry, config.BufferSize),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go async.flushLoop()
	return async
}

func (a *Async) Enabled(level Level) bool    { return a.logger.Enabled(level) }
func (a *Async) With(fields ...Field) Logger { return bind(a, fields) }
func (a *Async) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if !a.Enabled(level) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		return
	}
	select {
	case <-a.stop:
		a.mu.RUnlock()
		return
	default:
	}
	if level >= ErrorLevel {
		a.logger.Log(ctx, level, message, fields...)
		a.mu.RUnlock()
		return
	}
	entry := logEntry{ctx: ctx, level: level, msg: message, fields: slices.Clone(fields)}
	accepted := a.send(ctx, entry)
	a.mu.RUnlock()
	if !accepted {
		a.droppedCount.Add(1)
		if a.config.OnDropped != nil {
			a.config.OnDropped(level, message)
		}
	}
}

// send is called while admissions are locked. Close signals stop before
// acquiring the write lock, so a blocked publisher cannot deadlock shutdown.
func (a *Async) send(ctx context.Context, entry logEntry) bool {
	select {
	case a.buffer <- entry:
		return true
	default:
	}
	switch a.config.DropPolicy {
	case "block":
		select {
		case a.buffer <- entry:
			return true
		case <-ctx.Done():
			return false
		case <-a.stop:
			return false
		}
	case "sample":
		if a.fullCount.Add(1)%10 != 1 {
			return false
		}
		select {
		case a.buffer <- entry:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (a *Async) flushLoop() {
	defer close(a.done)
	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()
	batch := make([]logEntry, 0, 100)
	flush := func() {
		for _, entry := range batch {
			a.logger.Log(entry.ctx, entry.level, entry.msg, entry.fields...)
		}
		// Do not retain contexts, slices, or objects from the last batch.
		clear(batch)
		batch = batch[:0]
	}
	for {
		select {
		case entry, ok := <-a.buffer:
			if !ok {
				flush()
				return
			}
			if entry.synced != nil {
				flush()
				close(entry.synced)
				continue
			}
			batch = append(batch, entry)
			if len(batch) == cap(batch) {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (a *Async) Sync() error {
	barrier := make(chan struct{})
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()
		<-a.done
		return nil
	}
	select {
	case a.buffer <- logEntry{synced: barrier}:
		a.mu.RUnlock()
		<-barrier
		return syncLogger(a.logger)
	case <-a.stop:
		a.mu.RUnlock()
		<-a.done
		return nil
	}
}

func (a *Async) Close() error {
	a.once.Do(func() {
		close(a.stop)
		a.mu.Lock()
		a.closed = true
		close(a.buffer)
		a.mu.Unlock()
		<-a.done
		a.closeErr = closeLogger(a.logger)
	})
	return a.closeErr
}

func (a *Async) GetStats() (queueLength int64, droppedCount uint64) {
	return int64(len(a.buffer)), a.droppedCount.Load()
}

func (l *Async) presetFields() []Field { return presetFields(l.logger) }
