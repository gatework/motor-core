package logger

import (
	"context"
	"fmt"
	"os"
)

// Helper supplies formatted and level-specific conveniences over Logger.
// It carries fields through Logger.With and never stores a request context.
type Helper struct{ Logger }

func NewHelper(log Logger) *Helper {
	if log == nil {
		log = Default()
	}
	return &Helper{Logger: log}
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
func (h *Helper) Info(args ...any)                   { h.print(InfoLevel, args...) }
func (h *Helper) Infof(format string, args ...any)   { h.printf(InfoLevel, format, args...) }
func (h *Helper) Infow(message string, args ...any)  { h.printw(InfoLevel, message, args...) }
func (h *Helper) Trace(args ...any)                  { h.print(TraceLevel, args...) }
func (h *Helper) Tracef(format string, args ...any)  { h.printf(TraceLevel, format, args...) }
func (h *Helper) Debug(args ...any)                  { h.print(DebugLevel, args...) }
func (h *Helper) Debugf(format string, args ...any)  { h.printf(DebugLevel, format, args...) }
func (h *Helper) Debugw(message string, args ...any) { h.printw(DebugLevel, message, args...) }
func (h *Helper) Warn(args ...any)                   { h.print(WarnLevel, args...) }
func (h *Helper) Warnf(format string, args ...any)   { h.printf(WarnLevel, format, args...) }
func (h *Helper) Warnw(message string, args ...any)  { h.printw(WarnLevel, message, args...) }
func (h *Helper) Error(args ...any)                  { h.print(ErrorLevel, args...) }
func (h *Helper) Errorf(format string, args ...any)  { h.printf(ErrorLevel, format, args...) }
func (h *Helper) Errorw(message string, args ...any) { h.printw(ErrorLevel, message, args...) }
func (h *Helper) Fatal(args ...any)                  { h.print(FatalLevel, args...); os.Exit(1) }
func (h *Helper) Fatalf(format string, args ...any) {
	h.printf(FatalLevel, format, args...)
	os.Exit(1)
}
func (h *Helper) WithError(err error) *Helper { return NewHelper(h.With(FieldError(err))) }
func (h *Helper) WithFields(fields map[string]any) *Helper {
	return NewHelper(h.With(FieldsFromMap(fields)...))
}
