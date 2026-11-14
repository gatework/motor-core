package logger

import (
	"context"
	"fmt"
)

// Helper supplies formatted and level-specific conveniences over Logger.
// It carries fields through Logger.With and never stores a request context.
type Helper struct{ Logger }

// NewHelper wraps log with level-specific convenience methods and borrows its resources.
func NewHelper(log Logger) *Helper {
	if log == nil {
		log = Default()
	}
	return &Helper{Logger: log}
}

// Info records a message at info level.
func (h *Helper) Info(args ...any) { h.print(InfoLevel, args...) }

// Infof formats and records a message at info level.
func (h *Helper) Infof(format string, args ...any) { h.printf(InfoLevel, format, args...) }

// Infow records a info message with alternating field keys and values.
func (h *Helper) Infow(message string, args ...any) { h.printw(InfoLevel, message, args...) }

// Trace records a message at trace level.
func (h *Helper) Trace(args ...any) { h.print(TraceLevel, args...) }

// Tracef formats and records a message at trace level.
func (h *Helper) Tracef(format string, args ...any) { h.printf(TraceLevel, format, args...) }

// Debug records a message at debug level.
func (h *Helper) Debug(args ...any) { h.print(DebugLevel, args...) }

// Debugf formats and records a message at debug level.
func (h *Helper) Debugf(format string, args ...any) { h.printf(DebugLevel, format, args...) }

// Debugw records a debug message with alternating field keys and values.
func (h *Helper) Debugw(message string, args ...any) { h.printw(DebugLevel, message, args...) }

// Warn records a message at warning level.
func (h *Helper) Warn(args ...any) { h.print(WarnLevel, args...) }

// Warnf formats and records a message at warning level.
func (h *Helper) Warnf(format string, args ...any) { h.printf(WarnLevel, format, args...) }

// Warnw records a warning message with alternating field keys and values.
func (h *Helper) Warnw(message string, args ...any) { h.printw(WarnLevel, message, args...) }

// Error records a message at error level.
func (h *Helper) Error(args ...any) { h.print(ErrorLevel, args...) }

// Errorf formats and records a message at error level.
func (h *Helper) Errorf(format string, args ...any) { h.printf(ErrorLevel, format, args...) }

// Errorw records a error message with alternating field keys and values.
func (h *Helper) Errorw(message string, args ...any) { h.printw(ErrorLevel, message, args...) }

// Fatal records a fatal-level message; process termination belongs to the host.
func (h *Helper) Fatal(args ...any) { h.print(FatalLevel, args...) }

// Fatalf formats a fatal-level message; process termination belongs to the host.
func (h *Helper) Fatalf(format string, args ...any) {
	h.printf(FatalLevel, format, args...)
}

// WithError returns a helper with an error field, borrowing the same root.
func (h *Helper) WithError(err error) *Helper { return NewHelper(h.With(FieldError(err))) }

// WithFields returns a helper with a snapshot of the supplied field map.
func (h *Helper) WithFields(fields map[string]any) *Helper {
	return NewHelper(h.With(FieldsFromMap(fields)...))
}

func (h *Helper) print(level Level, args ...any) {
	if h.Enabled(level) {
		h.Log(context.Background(), level, fmt.Sprint(args...))
	}
}
func (h *Helper) printf(level Level, format string, args ...any) {
	if h.Enabled(level) {
		h.Log(context.Background(), level, fmt.Sprintf(format, args...))
	}
}
func (h *Helper) printw(level Level, message string, args ...any) {
	if !h.Enabled(level) {
		return
	}
	fields := make([]Field, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		if key, ok := args[i].(string); ok {
			fields = append(fields, Any(key, args[i+1]))
		}
	}
	h.Log(context.Background(), level, message, fields...)
}
