package logger

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoggerPipelineFields(t *testing.T) {
	constructors := map[string]func(...Option) (Logger, error){
		"text":   func(opts ...Option) (Logger, error) { return NewLogger(opts...), nil },
		"logrus": func(opts ...Option) (Logger, error) { return NewLogrusLogger(opts...) },
		"zap":    func(opts ...Option) (Logger, error) { return NewZapLogger(opts...) },
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			defaults := map[string]any{"password": "preset-secret", "scope": "base"}
			base, err := construct(WithOutput(&output), WithFields(defaults), WithEncoder("json"))
			if err != nil {
				t.Fatal(err)
			}
			defaults["scope"] = "mutated-scope"
			rules := append([]SanitizerRule{}, DefaultSanitizerRules...)
			rules = append(rules, SanitizerRule{FieldPattern: "request_id", Strategy: "remove"})
			sanitizer := NewSanitizerLogger(base, SanitizerConfig{Enabled: true, Rules: rules})
			rules[0].Strategy = "unknown"
			log := NewAsyncLogger(sanitizer, AsyncConfig{BufferSize: 8, FlushInterval: time.Hour, DropPolicy: "block"})
			child := log.With(FieldString("scope", "child")).With(FieldString("token", "typed-secret"))
			if _, ok := child.(io.Closer); ok {
				t.Fatal("a field view owns Close")
			}
			ctx := requestMetadataContext{Context: t.Context()}
			child.Log(ctx, InfoLevel, "entry", FieldString("phone", "13812345678"))
			NewHelper(child).Info(42)
			NewHelper(child).Infof("id=%d", 7)
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
			got := output.String()
			for _, secret := range []string{"preset-secret", "typed-secret", "request-secret", "13812345678", "mutated-scope"} {
				if strings.Contains(got, secret) {
					t.Errorf("entry leaked %q: %s", secret, got)
				}
			}
			for _, expected := range []string{"child", "sha256:", "138****5678", "42", "id=7"} {
				if !strings.Contains(got, expected) {
					t.Errorf("entry lost %q: %s", expected, got)
				}
			}
		})
	}
}

func TestAsyncOwnsEntryFields(t *testing.T) {
	var output bytes.Buffer
	base := NewLogger(WithOutput(&output))
	log := NewAsyncLogger(base, AsyncConfig{BufferSize: 4, FlushInterval: time.Hour, DropPolicy: "block"})
	fields := []Field{FieldString("value", "original")}
	log.Log(context.Background(), InfoLevel, "entry", fields...)
	fields[0].String = "mutated"
	preset := []Field{FieldString("preset", "original-preset")}
	child := log.With(preset...)
	preset[0].String = "mutated-preset"
	child.Log(context.Background(), InfoLevel, "child")
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "mutated") || !strings.Contains(got, "original") || !strings.Contains(got, "original-preset") {
		t.Fatalf("logger retained caller-owned slices: %s", got)
	}
}

func TestSamplingPreservesErrorEntries(t *testing.T) {
	var output bytes.Buffer
	log := NewSamplingLogger(NewLogger(WithOutput(&output)), SamplingConfig{Initial: 1, Thereafter: 100, Tick: time.Hour})
	child := log.With(FieldString("scope", "child"))
	for range 8 {
		child.Log(context.Background(), InfoLevel, "sampled")
	}
	child.Log(context.Background(), ErrorLevel, "typed error")
	NewHelper(child).Errorf("formatted error %d", 7)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "typed error") || !strings.Contains(got, "formatted error 7") {
		t.Fatalf("sampling dropped an error: %s", got)
	}
}

func TestDefaultReplacementBorrowsLogger(t *testing.T) {
	previous := Default()
	defer SetDefault(previous)
	var output bytes.Buffer
	first := NewLogger(WithOutput(&output))
	second := NewLogger(WithOutput(io.Discard))
	SetDefault(first)
	SetDefault(second)
	first.Log(context.Background(), InfoLevel, "still caller owned")
	if !strings.Contains(output.String(), "still caller owned") {
		t.Fatal("SetDefault closed the previous logger")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				SetDefault(second)
				Default().Log(context.Background(), InfoLevel, "concurrent")
			}
		})
	}
	wg.Wait()
}

func TestLoggerRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewLogrusLogger(WithAsync(AsyncConfig{DropPolicy: "invalid"})); err == nil {
		t.Fatal("invalid drop policy accepted")
	}
	if _, err := NewZapLogger(WithSanitizer(SanitizerConfig{Enabled: true, Rules: []SanitizerRule{{Strategy: "mask", KeepPrefix: -1}}})); err == nil {
		t.Fatal("negative mask prefix accepted")
	}
	cfg := DefaultConfig()
	cfg.Output.Console.Enabled = false
	cfg.Output.File.Path = "/dev/null/invalid.log"
	if _, err := NewFromConfig(cfg); err == nil {
		t.Fatal("unwritable file accepted")
	}
}

// A callback may re-enter the logger's lifecycle. It must run after publishing
// releases the admission lock, including when the buffer is already full.
func TestAsyncDropCallbackMayClose(t *testing.T) {
	var closed atomic.Bool
	var release sync.Once
	output := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	unblock := func() { release.Do(func() { close(output.release) }) }
	var log *Async
	log = NewAsyncLogger(NewLogger(WithOutput(output)), AsyncConfig{
		BufferSize: 1, FlushInterval: time.Millisecond,
		OnDropped: func(Level, string) { unblock(); _ = log.Close(); closed.Store(true) },
	})
	defer func() { unblock(); _ = log.Close() }()
	log.Log(context.Background(), InfoLevel, "block worker")
	select {
	case <-output.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start writing")
	}
	log.Log(context.Background(), InfoLevel, "fill buffer")
	done := make(chan struct{})
	go func() {
		log.Log(context.Background(), InfoLevel, "invoke callback")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("drop callback could not close the logger")
	}
	if !closed.Load() {
		t.Fatal("drop callback did not close the logger")
	}
}

// requestMetadataContext models external middleware that exposes the supported
// request_id metadata key without introducing a string-keyed context.WithValue.
type requestMetadataContext struct{ context.Context }

func (c requestMetadataContext) Value(key any) any {
	if key == "request_id" {
		return "request-secret"
	}
	return c.Context.Value(key)
}
