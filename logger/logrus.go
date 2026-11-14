package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"go.uber.org/zap/zapcore"
)

var loggerSourceDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return sourceDir(file)
}()

func sourceDir(file string) string { return filepath.ToSlash(filepath.Dir(file)) + "/" }

// Logrus is a Logrus adapter with an owned, optional rotating file output.
type Logrus struct {
	*root
	sink *logrusSink
}

type logrusSink struct {
	logger *logrus.Logger
	entry  *logrus.Entry
	opts   Options
	owned  io.Closer
	mu     sync.RWMutex
	closed bool
}

func NewLogrusLogger(opts ...Option) (*Logrus, error) {
	options := resolveOptions(opts)
	if err := validateOptions(options); err != nil {
		return nil, err
	}
	output, owned, err := openOutput(options)
	if err != nil {
		return nil, err
	}
	log := logrus.New()
	log.SetLevel(toLogrusLevel(options.Level))
	log.SetOutput(output)
	encoder := options.Encoder
	if encoder == "" {
		encoder = "text"
		if options.Path != "" && !options.Stdout {
			encoder = "json"
		}
	}
	if encoder == "json" {
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05.000",
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "time", logrus.FieldKeyLevel: "level",
				logrus.FieldKeyMsg: "msg", logrus.FieldKeyFunc: "caller",
			},
		})
	} else {
		log.SetFormatter(&ConsoleFormatter{TimestampFormat: "2006-01-02 15:04:05.000", ForceColors: true})
	}
	log.AddHook(&piiMaskHook{})
	entry := log.WithFields(logrus.Fields{})
	if options.Name != "" {
		entry = entry.WithField("logger", options.Name)
	}
	sink := &logrusSink{logger: log, entry: entry, opts: options, owned: owned}
	return &Logrus{root: newRoot(sink, options), sink: sink}, nil
}

func (l *logrusSink) Enabled(level Level) bool    { return l.logger.IsLevelEnabled(toLogrusLevel(level)) }
func (l *logrusSink) With(fields ...Field) Logger { return bind(l, fields) }
func (l *logrusSink) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if !l.Enabled(level) {
		return
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		return
	}
	entry := l.entry
	if ctx != nil {
		entry = entry.WithContext(ctx)
	}
	if metadata := contextFields(ctx); len(metadata) != 0 {
		fields = mergeFields(metadata, fields)
	}
	if len(fields) != 0 {
		entry = entry.WithFields(toLogrusFields(fields))
	}
	if l.opts.EnableCaller {
		entry = entry.WithField("caller", callerLocation(l.opts.CallerSkipCount))
	}
	// Log only records the entry. Process termination belongs to Helper.Fatal.
	entry.Log(toLogrusLevel(level), message)
}
func (l *logrusSink) Sync() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		return nil
	}
	return syncOutput(l.opts.Out)
}
func (l *logrusSink) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	err := syncOutput(l.opts.Out)
	if l.owned != nil {
		err = errors.Join(err, l.owned.Close())
	}
	return err
}

func shouldSkipFrame(file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return false
	}
	return strings.HasPrefix(file, loggerSourceDir) || strings.Contains(file, "/go/pkg/mod/") ||
		strings.Contains(file, "/runtime/") || strings.Contains(file, "/testing/")
}
func callerLocation(skip int) string {
	var pcs [16]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(max(3, skip), pcs[:])])
	for {
		frame, more := frames.Next()
		if !shouldSkipFrame(frame.File) {
			return formatCaller(frame)
		}
		if !more {
			return ""
		}
	}
}
func formatCaller(frame runtime.Frame) string { return fmt.Sprintf("%s:%d", frame.File, frame.Line) }

func toLogrusLevel(level Level) logrus.Level {
	switch level {
	case TraceLevel:
		return logrus.TraceLevel
	case DebugLevel:
		return logrus.DebugLevel
	case WarnLevel:
		return logrus.WarnLevel
	case ErrorLevel:
		return logrus.ErrorLevel
	case FatalLevel:
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}
func toLogrusFields(fields []Field) logrus.Fields {
	values := make(logrus.Fields, len(fields))
	for _, field := range fields {
		values[field.Key] = fieldValue(field)
	}
	return values
}

func (l *Logrus) AddHook(hook logrus.Hook)                { l.sink.logger.AddHook(hook) }
func (l *Logrus) GetLogrusLogger() *logrus.Logger         { return l.sink.logger }
func (l *Logrus) SetFormatter(formatter logrus.Formatter) { l.sink.logger.SetFormatter(formatter) }

// SentryHook Sentry 错误上报 Hook
type SentryHook struct {
	levels []logrus.Level
}

func NewSentryHook() *SentryHook {
	return &SentryHook{
		levels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
		},
	}
}

func (h *SentryHook) Levels() []logrus.Level {
	return h.levels
}

func (h *SentryHook) Fire(entry *logrus.Entry) error {
	// TODO: 集成 Sentry SDK
	// sentry.CaptureException(entry)
	return nil
}

// ElasticsearchHook Elasticsearch 日志存储 Hook
type ElasticsearchHook struct {
	client any // Elasticsearch client
	index  string
	levels []logrus.Level
}

func NewElasticsearchHook(client any, index string) *ElasticsearchHook {
	return &ElasticsearchHook{
		client: client,
		index:  index,
		levels: logrus.AllLevels,
	}
}

func (h *ElasticsearchHook) Levels() []logrus.Level {
	return h.levels
}

func (h *ElasticsearchHook) Fire(entry *logrus.Entry) error {
	// TODO: 将日志发送到 Elasticsearch
	// h.client.Index(h.index, entry)
	return nil
}

// CallerHook 修正调用者信息的 Hook（跳过 logger 包装层）
type CallerHook struct {
	skip int
}

func NewCallerHook(skip int) *CallerHook {
	return &CallerHook{skip: skip}
}

func (h *CallerHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *CallerHook) Fire(entry *logrus.Entry) error {
	// 获取真实的调用者信息
	// 从调用栈中查找第一个非 logger 包的调用者
	pcs := make([]uintptr, 30)
	depth := runtime.Callers(0, pcs)
	frames := runtime.CallersFrames(pcs[:depth])

	for {
		frame, more := frames.Next()
		// 跳过以下文件：
		// 1. logger 包内部文件（logrus.go, zap.go, default.go 等）
		// 2. logrus 库内部文件（entry.go, logger.go 等）
		// 3. runtime 包文件
		// 4. testing 包文件（仅在 Go test 运行时）
		if !strings.Contains(frame.File, "/logger/") &&
			!strings.Contains(frame.File, "/logrus@") &&
			!strings.Contains(frame.File, "/runtime/") &&
			!strings.Contains(frame.File, "/testing/") &&
			!strings.Contains(frame.Function, "github.com/gatework/motor-core/logger") &&
			!strings.Contains(frame.Function, "github.com/sirupsen/logrus") &&
			!strings.Contains(frame.Function, "runtime.") &&
			!strings.Contains(frame.Function, "testing.") {
			// 找到第一个业务代码调用位置
			entry.Caller = &runtime.Frame{
				PC:       frame.PC,
				File:     frame.File,
				Line:     frame.Line,
				Function: frame.Function,
			}
			break
		}
		if !more {
			break
		}
	}
	return nil
}

// MetricsHook Prometheus 监控 Hook
type MetricsHook struct {
	// TODO: Prometheus metrics
	levels []logrus.Level
}

func NewMetricsHook() *MetricsHook {
	return &MetricsHook{
		levels: logrus.AllLevels,
	}
}

func (h *MetricsHook) Levels() []logrus.Level {
	return h.levels
}

func (h *MetricsHook) Fire(entry *logrus.Entry) error {
	// TODO: 记录 Prometheus 指标
	// logCounter.WithLabelValues(entry.Level.String()).Inc()
	return nil
}

func callerFrame(skip int) zapcore.EntryCaller {
	var pcs [16]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(max(3, skip), pcs[:])])
	for {
		frame, more := frames.Next()
		if !shouldSkipFrame(frame.File) {
			return zapcore.EntryCaller{Defined: true, PC: frame.PC, File: frame.File, Line: frame.Line, Function: frame.Function}
		}
		if !more {
			return zapcore.EntryCaller{}
		}
	}
}
