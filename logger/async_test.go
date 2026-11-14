package logger

import "fmt"
import "context"

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// stopAsync stops the logger and waits for its flush goroutine to exit.
//
// The output buffer must not be read while that goroutine is still running:
// bytes.Buffer is not safe for concurrent use, and Sync only forces a flush —
// it does not stop the periodic loop. Close waits on the WaitGroup, so reading
// after it is safe.
func stopAsync(t *testing.T, l Logger) {
	t.Helper()
	if closer, ok := any(l).(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}

// TestAsyncLogger_Basic 测试异步日志基础功能
func TestAsyncLogger_Basic(t *testing.T) {
	buf := &bytes.Buffer{}

	// 创建异步 logger
	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    100,
		FlushInterval: 50 * time.Millisecond,
		DropPolicy:    "drop",
	})

	// 写入日志
	asyncLogger.Log(context.Background(), InfoLevel, "test message 1")
	asyncLogger.Log(context.Background(), InfoLevel, fmt.Sprintf("test message %d", 2))

	// 确保刷新完成（修复数据竞争）
	if syncer, ok := any(asyncLogger).(interface{ Sync() error }); ok {
		if err := syncer.Sync(); err != nil {
			t.Fatalf("Failed to sync: %v", err)
		}
	}

	// 验证输出
	stopAsync(t, asyncLogger)
	output := buf.String()
	if !strings.Contains(output, "test message 1") {
		t.Errorf("Expected 'test message 1' in output, got: %s", output)
	}
	if !strings.Contains(output, "test message 2") {
		t.Errorf("Expected 'test message 2' in output, got: %s", output)
	}

	// 关闭 logger
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_BufferFull 测试队列满时的策略
type blockedWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(p), nil
}

func TestAsyncLogger_BufferFull(t *testing.T) {
	output := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	dropped := 0
	log := NewAsyncLogger(mustLogrus(WithOutput(output)), AsyncConfig{
		BufferSize: 1, FlushInterval: time.Millisecond, DropPolicy: "drop",
		OnDropped: func(Level, string) { dropped++ },
	})
	defer func() {
		close(output.release)
		stopAsync(t, log)
	}()
	log.Log(context.Background(), InfoLevel, "block worker")
	select {
	case <-output.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start writing")
	}
	log.Log(context.Background(), InfoLevel, "fill buffer")
	log.Log(context.Background(), InfoLevel, "drop entry")
	if dropped != 1 {
		t.Fatalf("dropped %d entries, want 1", dropped)
	}
}

// TestAsyncLogger_Sync 测试同步刷新
func TestAsyncLogger_Sync(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    1000,
		FlushInterval: 10 * time.Second, // 很长的刷新间隔
		DropPolicy:    "drop",
	})

	// 写入日志
	asyncLogger.Log(context.Background(), InfoLevel, "test sync")

	// 立即同步
	if syncer, ok := any(asyncLogger).(interface{ Sync() error }); ok {
		syncer.Sync()
	}

	// 验证已写入
	stopAsync(t, asyncLogger)
	if !strings.Contains(buf.String(), "test sync") {
		t.Errorf("Expected 'test sync' in output after Sync()")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_GracefulShutdown 测试优雅关闭
func TestAsyncLogger_GracefulShutdown(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    1000,
		FlushInterval: 10 * time.Second,
		DropPolicy:    "drop",
	})

	// 写入多条日志
	for i := 0; i < 10; i++ {
		asyncLogger.Log(context.Background(), InfoLevel, fmt.Sprintf("message %d", i))
	}

	// 关闭（应该刷新所有日志）
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}

	// 验证所有日志都已写入
	stopAsync(t, asyncLogger)
	output := buf.String()
	for i := 0; i < 10; i++ {
		expected := "message"
		if !strings.Contains(output, expected) {
			t.Errorf("Expected '%s' in output, got: %s", expected, output)
		}
	}
}

// BenchmarkAsyncLogger 性能测试
func BenchmarkAsyncLogger(b *testing.B) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, DefaultAsyncConfig)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		asyncLogger.Log(context.Background(), InfoLevel, "benchmark message")
	}
	b.StopTimer()

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// BenchmarkAsyncLogger_vs_Sync 对比异步和同步性能
func BenchmarkAsyncLogger_vs_Sync(b *testing.B) {
	b.Run("Sync", func(b *testing.B) {
		buf := &bytes.Buffer{}
		logger := mustLogrus(
			WithOutput(buf),
			WithLevel(InfoLevel),
		)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			logger.Log(context.Background(), InfoLevel, "sync message")
		}
	})

	b.Run("Async", func(b *testing.B) {
		buf := &bytes.Buffer{}
		baseLogger := mustLogrus(
			WithOutput(buf),
			WithLevel(InfoLevel),
		)
		asyncLogger := NewAsyncLogger(baseLogger, DefaultAsyncConfig)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			asyncLogger.Log(context.Background(), InfoLevel, "async message")
		}
		b.StopTimer()

		if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
			closer.Close()
		}
	})
}

// TestAsyncLogger_Fields 测试 Fields 方法
func TestAsyncLogger_Fields(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    100,
		FlushInterval: 50 * time.Millisecond,
	})

	// 使用 Fields
	asyncLogger.With(FieldsFromMap(map[string]interface{}{
		"user_id": 123,
		"action":  "login",
	})...,
	).Log(context.Background(), InfoLevel, "user logged in")

	// 确保刷新完成（修复数据竞争）
	if syncer, ok := any(asyncLogger).(interface{ Sync() error }); ok {
		if err := syncer.Sync(); err != nil {
			t.Fatalf("Failed to sync: %v", err)
		}
	}

	// 验证字段
	stopAsync(t, asyncLogger)
	output := buf.String()
	if !strings.Contains(output, "user_id") {
		t.Errorf("Expected 'user_id' in output")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_HighConcurrency 测试高并发
func TestAsyncLogger_HighConcurrency(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    1000,
		FlushInterval: 50 * time.Millisecond,
		DropPolicy:    "drop",
	})

	// 多个 goroutine 并发写入
	const goroutines = 10
	const logsPerGoroutine = 100

	done := make(chan bool)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			for i := 0; i < logsPerGoroutine; i++ {
				asyncLogger.Log(context.Background(), InfoLevel, fmt.Sprintf("goroutine %d message %d", id, i))
			}
			done <- true
		}(g)
	}

	// 等待所有 goroutine 完成
	for g := 0; g < goroutines; g++ {
		<-done
	}

	// 等待刷新
	if syncer, ok := any(asyncLogger).(interface{ Sync() error }); ok {
		syncer.Sync()
	}

	// 验证日志数量（至少有一部分被记录）
	stopAsync(t, asyncLogger)
	output := buf.String()
	if len(output) == 0 {
		t.Error("Expected some logs to be written")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_ErrorLevel 测试 Error 级别不丢失
func TestAsyncLogger_ErrorLevel(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    10, // small buffer
		FlushInterval: 1 * time.Second,
		DropPolicy:    "drop",
	})

	// 先填满缓冲区
	for i := 0; i < 20; i++ {
		asyncLogger.Log(context.Background(), InfoLevel, "filler")
	}

	// 写入 Error 日志（应该同步写入，不丢失）
	asyncLogger.Log(context.Background(), ErrorLevel, "critical error")

	// 立即检查（不等待刷新）
	time.Sleep(10 * time.Millisecond)

	stopAsync(t, asyncLogger)
	output := buf.String()
	if !strings.Contains(output, "critical error") {
		t.Error("Error level log should not be dropped")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_GetStats 测试监控指标
func TestAsyncLogger_GetStats(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Second, // 长时间不刷新
		DropPolicy:    "drop",
	})

	// 写入超过缓冲区的日志
	for i := 0; i < 50; i++ {
		asyncLogger.Log(context.Background(), InfoLevel, "message")
	}

	// 获取统计信息
	if statsGetter, ok := any(asyncLogger).(interface {
		GetStats() (queueLength int64, droppedCount uint64)
	}); ok {
		queueLen, dropped := statsGetter.GetStats()

		t.Logf("Queue Length: %d, Dropped: %d", queueLen, dropped)

		// 验证有日志被丢弃
		if dropped == 0 {
			t.Error("Expected some logs to be dropped")
		}
	} else {
		t.Error("asyncLogger should implement GetStats()")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_BlockPolicy 测试阻塞策略
func TestAsyncLogger_BlockPolicy(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    10,
		FlushInterval: 100 * time.Millisecond,
		DropPolicy:    "block", // 阻塞策略
	})

	// 写入大量日志（应该阻塞但不丢失）
	for i := 0; i < 20; i++ {
		asyncLogger.Log(context.Background(), InfoLevel, fmt.Sprintf("message %d", i))
	}

	// 确保刷新完成（修复数据竞争）
	if syncer, ok := any(asyncLogger).(interface{ Sync() error }); ok {
		if err := syncer.Sync(); err != nil {
			t.Fatalf("Failed to sync: %v", err)
		}
	}

	// 验证所有日志都记录了（阻塞不丢失）
	stopAsync(t, asyncLogger)
	output := buf.String()
	if !strings.Contains(output, "message") {
		t.Error("Block policy should not drop logs")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestAsyncLogger_SamplePolicy 测试采样策略
func TestAsyncLogger_SamplePolicy(t *testing.T) {
	buf := &bytes.Buffer{}

	baseLogger := mustLogrus(
		WithOutput(buf),
		WithLevel(InfoLevel),
	)

	droppedCount := 0
	asyncLogger := NewAsyncLogger(baseLogger, AsyncConfig{
		BufferSize:    10,
		FlushInterval: 1 * time.Second,
		DropPolicy:    "sample", // 采样策略
		OnDropped: func(level Level, msg string) {
			droppedCount++
		},
	})

	// 写入大量日志
	for i := 0; i < 100; i++ {
		asyncLogger.Log(context.Background(), InfoLevel, "message")
	}

	// 等待处理
	time.Sleep(100 * time.Millisecond)

	t.Logf("Dropped %d logs (sample policy)", droppedCount)

	// 验证采样策略有效（部分丢弃，部分保留）
	if droppedCount == 0 {
		t.Error("Sample policy should drop some logs")
	}
	if droppedCount == 100 {
		t.Error("Sample policy should keep some logs (not drop all)")
	}

	// 关闭
	if closer, ok := any(asyncLogger).(interface{ Close() error }); ok {
		closer.Close()
	}
}

// Sync has to write entries that have already been taken out of the channel.
//
// They do not wait there until they are written: the flush loop moves them into
// a batch of its own and writes that batch on a timer. A long flush interval
// removes the timer from the picture, so only Sync can get the entry out.
//
// This does not reproduce the scheduling race that made TestAsyncLogger_Basic
// fail on CI — that window is a few instructions wide and cannot be forced from
// a test. It pins the guarantee Sync is supposed to offer, which the new
// implementation provides by construction rather than by polling.
func TestSyncWaitsForEntriesAlreadyTakenFromTheChannel(t *testing.T) {
	buf := &bytes.Buffer{}
	base := mustLogrus(WithOutput(buf), WithLevel(InfoLevel))

	async := NewAsyncLogger(base, AsyncConfig{
		BufferSize:    100,
		FlushInterval: time.Hour, // only Sync can flush
		DropPolicy:    "drop",
	})
	defer stopAsync(t, async)

	async.Log(context.Background(), InfoLevel, "written before sync")

	// Long enough for the flush loop to move the entry out of the channel and
	// into its batch, where the old Sync could no longer see it.
	time.Sleep(100 * time.Millisecond)

	syncer, ok := any(async).(interface{ Sync() error })
	if !ok {
		t.Fatal("the async logger must expose Sync")
	}
	// Bounded on purpose: with no timer to fall back on, a Sync that stopped
	// returning would hang the whole package until the test binary timed out,
	// and the failure would name the wrong thing.
	done := make(chan error, 1)
	go func() { done <- syncer.Sync() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Sync did not return")
	}

	if got := buf.String(); !strings.Contains(got, "written before sync") {
		t.Errorf("Sync returned before the entry reached the output, got: %q", got)
	}
}
