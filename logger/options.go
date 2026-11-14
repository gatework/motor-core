package logger

import (
	"fmt"
	"io"
	"slices"
)

// Option is a function that configures the logger options
// Option 是一个配置日志记录器选项的函数
type Option func(*Options)

// Options are logger options
// Options 是日志记录器选项
type Options struct {
	// Level is the logging level the logger should log at. default is `InfoLevel`
	// Level 是日志记录器的日志级别，默认是 `InfoLevel`
	Level Level
	// Fields are fields to always be logged
	// Fields 是始终要记录的字段
	Fields map[string]interface{}
	// Out is the output writer for the logger. default is `os.Stderr`
	// Out 是日志记录器的输出写入器，默认是 `os.Stderr`
	Out io.Writer
	// CallerSkipCount is the frame count to skip for file:line info
	// CallerSkipCount 是跳过的帧数，用于文件:行信息
	CallerSkipCount int
	// Optional decorators are configured explicitly, without context values.
	Async     *AsyncConfig
	Sampling  *SamplingConfig
	Sanitizer *SanitizerConfig
	// Name is the logger name
	// Name 是日志记录器的名称
	Name string
	// Path is the log file path (optional)
	// Path 是日志文件路径（可选）
	Path string
	// Stdout enables console output
	// Stdout 启用控制台输出
	Stdout bool

	// Encoder 日志格式: json（文件）, text（控制台彩色）, console
	Encoder string
	// EnableCaller 是否启用调用者信息（文件名:行号）
	EnableCaller     bool
	EnableStacktrace bool

	// --- 日志轮转配置 ---
	// MaxSize 单个文件最大大小（MB），默认 100
	MaxSize int
	// MaxAge 文件最大保留天数，默认 30
	MaxAge int
	// MaxBackups 最大备份数量，默认 7
	MaxBackups int
	// Compress 是否压缩旧日志，默认 true
	Compress bool
	// LocalTime 使用本地时间（而非 UTC），默认 true
	LocalTime bool
}

// WithFields sets default fields for the logger
// WithFields 设置日志记录器的默认字段
func WithFields(fields map[string]interface{}) Option {
	return func(args *Options) {
		args.Fields = copyFields(fields)
	}
}

// WithLevel sets default level for the logger
// WithLevel 设置日志记录器的默认级别
func WithLevel(level Level) Option {
	return func(args *Options) {
		args.Level = level
	}
}

// WithOutput sets default output writer for the logger
// WithOutput 设置日志记录器的默认输出写入器
func WithOutput(out io.Writer) Option {
	return func(args *Options) {
		args.Out = out
	}
}

// WithCallerSkipCount sets frame count to skip
// WithCallerSkipCount 设置要跳过的帧数
func WithCallerSkipCount(c int) Option {
	return func(args *Options) {
		args.CallerSkipCount = c
	}
}

// WithName sets name for logger
// WithName 设置日志记录器的名称
func WithName(name string) Option {
	return func(args *Options) {
		args.Name = name
	}
}

// WithStdout sets whether to enable console output
// WithStdout 设置是否启用控制台输出
func WithStdout(enabled bool) Option {
	return func(args *Options) {
		args.Stdout = enabled
	}
}

// WithPath sets the log file path
// WithPath 设置日志文件路径
func WithPath(path string) Option {
	return func(args *Options) {
		args.Path = path
	}
}

// WithEncoder sets the log encoder format (json, text, console)
// WithEncoder 设置日志格式（json、text、console）
func WithEncoder(encoder string) Option {
	return func(args *Options) {
		args.Encoder = encoder
	}
}

// WithEnableCaller sets whether to enable caller info (file:line)
// WithEnableCaller 设置是否启用调用者信息（文件名:行号）
func WithEnableCaller(enabled bool) Option {
	return func(args *Options) {
		args.EnableCaller = enabled
	}
}

// WithRotation sets log rotation config
// WithRotation 设置日志轮转配置
func WithRotation(maxSize, maxAge, maxBackups int, compress bool) Option {
	return func(args *Options) {
		if maxSize > 0 {
			args.MaxSize = maxSize
		}
		if maxAge > 0 {
			args.MaxAge = maxAge
		}
		if maxBackups > 0 {
			args.MaxBackups = maxBackups
		}
		args.Compress = compress
	}
}

// WithSampling sets sampling configuration for the logger
// WithSampling 设置日志采样配置
func WithSampling(config SamplingConfig) Option {
	return func(o *Options) {
		copy := config
		copy.ExcludeLevels = slices.Clone(config.ExcludeLevels)
		o.Sampling = &copy
	}
}

// WithAsync sets async configuration for the logger
// WithAsync 设置异步日志配置
func WithAsync(config AsyncConfig) Option {
	return func(o *Options) {
		copy := config
		o.Async = &copy
	}
}

// WithSanitizer sets sanitizer configuration for the logger
// WithSanitizer 设置脱敏配置
func WithSanitizer(config SanitizerConfig) Option {
	return func(o *Options) {
		copy := config
		copy.Rules = slices.Clone(config.Rules)
		o.Sanitizer = &copy
	}
}

// DefaultOptions returns default options
// DefaultOptions 返回默认选项
func DefaultOptions() Options {
	return Options{
		Level:           InfoLevel,
		Fields:          make(map[string]interface{}),
		Out:             nil,
		CallerSkipCount: 3,
		Name:            "",
		MaxSize:         100,
		MaxAge:          30,
		MaxBackups:      7,
		Compress:        true,
		LocalTime:       true,
	}
}

func resolveOptions(opts []Option) Options {
	options := DefaultOptions()
	for _, option := range opts {
		if option != nil {
			option(&options)
		}
	}
	options.Fields = copyFields(options.Fields)
	return options
}

func WithEnableStacktrace(enabled bool) Option {
	return func(opts *Options) { opts.EnableStacktrace = enabled }
}

func validateOptions(opts Options) error {
	if async := opts.Async; async != nil {
		if async.BufferSize < 0 || async.FlushInterval < 0 {
			return fmt.Errorf("logger: async values must be nonnegative")
		}
		switch async.DropPolicy {
		case "", "drop", "block", "sample":
		default:
			return fmt.Errorf("logger: invalid async drop policy %q", async.DropPolicy)
		}
	}
	if sampling := opts.Sampling; sampling != nil {
		if sampling.Initial < 0 || sampling.Thereafter < 0 || sampling.Tick < 0 {
			return fmt.Errorf("logger: sampling values must be nonnegative")
		}
	}
	if sanitizer := opts.Sanitizer; sanitizer != nil {
		for _, rule := range sanitizer.Rules {
			if rule.KeepPrefix < 0 || rule.KeepSuffix < 0 {
				return fmt.Errorf("logger: mask lengths must be nonnegative")
			}
			switch rule.Strategy {
			case "mask", "hash", "remove":
			default:
				return fmt.Errorf("logger: invalid sanitizer strategy %q", rule.Strategy)
			}
		}
	}
	return nil
}
