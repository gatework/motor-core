package logger

import (
	"context"
	"slices"
	"sync"
	"time"
)

// SamplingConfig 采样配置
type SamplingConfig struct {
	ExcludeLevels []Level
	// Initial 每个 Tick 周期内前 N 条日志必须记录
	Initial int
	// Thereafter 超过 Initial 后，每 M 条记录 1 条
	Thereafter int
	// Tick 采样周期（例如：每秒、每分钟重置计数器）
	Tick time.Duration
}

// DefaultSamplingConfig 默认采样配置（生产环境推荐）
var DefaultSamplingConfig = SamplingConfig{
	Initial:    10,          // 每秒前 10 条必记录
	Thereafter: 100,         // 之后每 100 条记录 1 条
	Tick:       time.Second, // 每秒重置
}

// Sampling applies one shared sampling budget to its root and field views.
// Error and Fatal entries always bypass sampling.
type Sampling struct {
	logger   Logger
	config   SamplingConfig
	mu       sync.Mutex
	counter  uint64
	start    time.Time
	once     sync.Once
	closeErr error
}

func NewSamplingLogger(log Logger, config SamplingConfig) *Sampling {
	if log == nil {
		log = Default()
	}
	if config.Initial <= 0 {
		config.Initial = DefaultSamplingConfig.Initial
	}
	if config.Thereafter <= 0 {
		config.Thereafter = DefaultSamplingConfig.Thereafter
	}
	if config.Tick <= 0 {
		config.Tick = DefaultSamplingConfig.Tick
	}
	config.ExcludeLevels = slices.Clone(config.ExcludeLevels)
	return &Sampling{logger: log, config: config, start: time.Now()}
}
func (s *Sampling) Enabled(level Level) bool    { return s.logger.Enabled(level) }
func (s *Sampling) With(fields ...Field) Logger { return bind(s, fields) }
func (s *Sampling) Log(ctx context.Context, level Level, message string, fields ...Field) {
	if s.Enabled(level) && (level >= ErrorLevel || slices.Contains(s.config.ExcludeLevels, level) || s.shouldSample()) {
		s.logger.Log(ctx, level, message, fields...)
	}
}
func (s *Sampling) shouldSample() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.Sub(s.start) >= s.config.Tick {
		s.start = now
		s.counter = 0
	}
	s.counter++
	initial := uint64(s.config.Initial)
	return s.counter <= initial || (s.counter-initial-1)%uint64(s.config.Thereafter) == 0
}
func (s *Sampling) Sync() error { return syncLogger(s.logger) }
func (s *Sampling) Close() error {
	s.once.Do(func() { s.closeErr = closeLogger(s.logger) })
	return s.closeErr
}

func (l *Sampling) presetFields() []Field { return presetFields(l.logger) }
