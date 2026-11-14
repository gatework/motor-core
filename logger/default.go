package logger

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"sync"
	"time"
)

// Text writes entries using the package's compact text format.
// WithOutput writers are borrowed; Close never closes them.
type Text struct{ *root }

type textSink struct {
	mu     sync.Mutex
	opts   Options
	closed bool
}

func newDefault() *Text {
	level, err := GetLevel(os.Getenv("GO_ADMIN_LOG_LEVEL"))
	if err != nil {
		level = InfoLevel
	}
	return NewLogger(WithLevel(level))
}

func NewLogger(opts ...Option) *Text {
	options := resolveOptions(opts)
	if options.Out == nil {
		options.Out = os.Stderr
	}
	sink := &textSink{opts: options}
	return &Text{root: newRoot(sink, options)}
}

func (l *textSink) Enabled(level Level) bool    { return l.opts.Level.Enabled(level) }
func (l *textSink) With(fields ...Field) Logger { return bind(l, fields) }
func (l *textSink) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if !l.Enabled(level) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	fields = mergeFields(contextFields(ctx), fields)
	if mask := GetMasker(); mask != nil {
		message = mask(message)
		fields = maskEntryFields(fields, mask)
	}
	var line strings.Builder
	if l.opts.Name != "" {
		fmt.Fprintf(&line, "[%s] ", l.opts.Name)
	}
	fmt.Fprintf(&line, "%s %s %s", time.Now().Format("2006-01-02 15:04:05.000Z0700"), level.String(), message)
	for _, field := range fields {
		fmt.Fprintf(&line, " %s=%v", field.Key, fieldValue(field))
	}
	line.WriteByte('\n')
	_, _ = io.WriteString(l.opts.Out, line.String())
}
func (l *textSink) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return syncOutput(l.opts.Out)
}
func (l *textSink) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return syncOutput(l.opts.Out)
}

func syncOutput(out io.Writer) error {
	if out == os.Stdout || out == os.Stderr {
		return nil
	}
	if syncer, ok := out.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}
	return nil
}

func copyFields(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	maps.Copy(dst, src)
	return dst
}
