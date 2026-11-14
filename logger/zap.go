package logger

import (
	"context"
	"errors"
	"io"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Zap is a Zap adapter with an owned, optional rotating file output.
type Zap struct{ *root }

type zapSink struct {
	core   zapcore.Core
	opts   Options
	owned  io.Closer
	mu     sync.RWMutex
	closed bool
}

func NewZapLogger(opts ...Option) (*Zap, error) {
	options := resolveOptions(opts)
	if err := validateOptions(options); err != nil {
		return nil, err
	}
	output, owned, err := openOutput(options)
	if err != nil {
		return nil, err
	}
	config := zapcore.EncoderConfig{
		TimeKey: "time", LevelKey: "level", NameKey: "logger", CallerKey: "caller",
		MessageKey: "msg", StacktraceKey: "stacktrace", LineEnding: zapcore.DefaultLineEnding,
		EncodeLevel: zapcore.LowercaseLevelEncoder, EncodeTime: zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder, EncodeCaller: zapcore.ShortCallerEncoder,
	}
	var encoder zapcore.Encoder = zapcore.NewJSONEncoder(config)
	if options.Stdout || options.Encoder == "console" || options.Encoder == "text" {
		config.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(config)
	}
	// Synchronize even custom writers, which need not be safe for concurrent use.
	writer := zapcore.Lock(zapcore.AddSync(output))
	core := newMaskingCore(zapcore.NewCore(encoder, writer, toZapLevel(options.Level)))
	sink := &zapSink{core: core, opts: options, owned: owned}
	return &Zap{root: newRoot(sink, options)}, nil
}
func (z *zapSink) Enabled(level Level) bool    { return z.core.Enabled(toZapLevel(level)) }
func (z *zapSink) With(fields ...Field) Logger { return bind(z, fields) }
func (z *zapSink) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if !z.Enabled(level) {
		return
	}
	z.mu.RLock()
	defer z.mu.RUnlock()
	if z.closed {
		return
	}
	if metadata := contextFields(ctx); len(metadata) != 0 {
		fields = mergeFields(metadata, fields)
	}
	entry := zapcore.Entry{Level: toZapLevel(level), Time: time.Now(), Message: message, LoggerName: z.opts.Name}
	if z.opts.EnableCaller {
		// Use the same wrapper-aware caller selection as the Logrus adapter.
		entry.Caller = callerFrame(z.opts.CallerSkipCount)
	}
	if z.opts.EnableStacktrace && level >= ErrorLevel {
		entry.Stack = string(debug.Stack())
	}
	_ = z.core.Write(entry, toZapFields(fields))
}
func (z *zapSink) Sync() error {
	z.mu.RLock()
	defer z.mu.RUnlock()
	if z.closed {
		return nil
	}
	// Standard terminal descriptors may reject fsync. Our own rotating output
	// has no userspace buffer; only synchronize explicitly supplied writers.
	return syncOutput(z.opts.Out)
}
func (z *zapSink) Close() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.closed {
		return nil
	}
	z.closed = true
	err := syncOutput(z.opts.Out)
	if z.owned != nil {
		err = errors.Join(err, z.owned.Close())
	}
	return err
}

func toZapLevel(level Level) zapcore.Level {
	switch level {
	case TraceLevel, DebugLevel:
		return zapcore.DebugLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	case FatalLevel:
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}
func toZapFields(fields []Field) []zap.Field {
	converted := make([]zap.Field, len(fields))
	for i, field := range fields {
		switch field.Type {
		case FieldTypeString:
			converted[i] = zap.String(field.Key, field.String)
		case FieldTypeInt:
			converted[i] = zap.Int64(field.Key, field.Int64)
		case FieldTypeBool:
			converted[i] = zap.Bool(field.Key, field.Int64 != 0)
		case FieldTypeDuration:
			converted[i] = zap.Duration(field.Key, time.Duration(field.Int64))
		case FieldTypeTime:
			converted[i] = zap.Time(field.Key, time.Unix(0, field.Int64))
		case FieldTypeError:
			if err, ok := field.Any.(error); ok {
				converted[i] = zap.NamedError(field.Key, err)
			} else {
				converted[i] = zap.Any(field.Key, field.Any)
			}
		default:
			converted[i] = zap.Any(field.Key, field.Any)
		}
	}
	return converted
}
