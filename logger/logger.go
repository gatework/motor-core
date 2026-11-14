package logger

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
)

// Logger records structured entries. Context belongs to the individual call;
// implementations must not retain it beyond processing that entry.
type Logger interface {
	Log(ctx context.Context, level Level, message string, fields ...Field)
	With(fields ...Field) Logger
	Enabled(level Level) bool
}

var defaultState = struct {
	sync.RWMutex
	logger Logger
}{}

var defaultOnce sync.Once

// Default returns the current process logger.
func Default() Logger {
	defaultOnce.Do(func() {
		defaultState.Lock()
		defer defaultState.Unlock()
		if defaultState.logger == nil {
			defaultState.logger = newDefault()
		}
	})
	defaultState.RLock()
	log := defaultState.logger
	defaultState.RUnlock()
	return log
}

// SetDefault publishes a logger. The caller owns both its lifetime and that of
// the previous logger; publishing does not close either one. Nil resets it.
func SetDefault(log Logger) {
	if log == nil {
		log = newDefault()
	}
	defaultState.Lock()
	defaultState.logger = log
	defaultState.Unlock()
}

func Log(ctx context.Context, level Level, message string, fields ...Field) {
	Default().Log(ctx, level, message, fields...)
}

func With(fields ...Field) Logger { return Default().With(fields...) }

func Logf(level Level, format string, args ...any) {
	log := Default()
	if log.Enabled(level) {
		log.Log(context.Background(), level, fmt.Sprintf(format, args...))
	}
}

// boundLogger carries immutable fields and borrows its root's resources.
// With never creates a worker or transfers Close/Sync ownership to a child.
type boundLogger struct {
	logger Logger
	fields []Field
}

func bind(log Logger, fields []Field) Logger {
	if len(fields) == 0 {
		return log
	}
	return &boundLogger{logger: log, fields: slices.Clone(fields)}
}

func (b *boundLogger) Enabled(level Level) bool { return b.logger.Enabled(level) }
func (b *boundLogger) With(fields ...Field) Logger {
	return &boundLogger{logger: b.logger, fields: mergeFields(b.fields, fields)}
}
func (b *boundLogger) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if b.Enabled(level) {
		b.logger.Log(ctx, level, message, mergeFields(b.fields, fields)...)
	}
}

// mergeFields preserves field order and lets the later container override keys.
// In particular, a sanitized field must replace a prebound raw value rather
// than create duplicate JSON keys containing both forms.
func mergeFields(first, second []Field) []Field {
	if len(first) == 0 {
		return second
	}
	if len(second) == 0 {
		return first
	}
	fields := make([]Field, 0, len(first)+len(second))
	for _, field := range first {
		overridden := false
		for _, replacement := range second {
			if field.Key == replacement.Key {
				overridden = true
				break
			}
		}
		if !overridden {
			fields = append(fields, field)
		}
	}
	return append(fields, second...)
}

func presetFields(log Logger) []Field {
	if preset, ok := log.(interface{ presetFields() []Field }); ok {
		return preset.presetFields()
	}
	return nil
}

func (b *boundLogger) presetFields() []Field {
	return mergeFields(presetFields(b.logger), b.fields)
}

// root owns an adapter pipeline. Only the value returned by a constructor
// exposes lifetime operations; values returned by With only borrow it.
type root struct {
	Logger
	owner    Logger
	fields   []Field
	once     sync.Once
	closeErr error
}

func newRoot(output Logger, opts Options) *root {
	pipeline := output
	if opts.Sanitizer != nil {
		pipeline = NewSanitizerLogger(pipeline, *opts.Sanitizer)
	}
	if opts.Sampling != nil {
		pipeline = NewSamplingLogger(pipeline, *opts.Sampling)
	}
	if opts.Async != nil {
		pipeline = NewAsyncLogger(pipeline, *opts.Async)
	}
	fields := FieldsFromMap(opts.Fields)
	return &root{Logger: bind(pipeline, fields), owner: pipeline, fields: fields}
}
func (r *root) Close() error {
	r.once.Do(func() { r.closeErr = closeLogger(r.owner) })
	return r.closeErr
}
func (r *root) Sync() error { return syncLogger(r.owner) }

func closeLogger(log Logger) error {
	if closer, ok := log.(io.Closer); ok {
		return closer.Close()
	}
	return syncLogger(log)
}
func syncLogger(log Logger) error {
	if syncer, ok := log.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}
	return nil
}

func (r *root) presetFields() []Field { return r.fields }
