package logger

import (
	"fmt"
	"regexp"
	"slices"
	"sync"
	"time"
)

// AdapterFactory creates a resource-owning logger or reports construction failure.
type AdapterFactory func(opts ...Option) (Logger, error)

var registryMu sync.RWMutex
var adapterRegistry = map[string]AdapterFactory{
	"default": func(opts ...Option) (Logger, error) { return NewLogger(opts...), nil },
	"logrus": func(opts ...Option) (Logger, error) {
		log, err := NewLogrusLogger(opts...)
		if err != nil {
			return nil, err
		}
		return log, nil
	},
	"zap": func(opts ...Option) (Logger, error) {
		log, err := NewZapLogger(opts...)
		if err != nil {
			return nil, err
		}
		return log, nil
	},
}

func RegisterAdapter(name string, factory AdapterFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	adapterRegistry[name] = factory
}
func GetAdapter(name string) (AdapterFactory, error) {
	registryMu.RLock()
	factory := adapterRegistry[name]
	registryMu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("logger: adapter %q not found", name)
	}
	return factory, nil
}
func ListAdapters() []string {
	registryMu.RLock()
	names := make([]string, 0, len(adapterRegistry))
	for name := range adapterRegistry {
		names = append(names, name)
	}
	registryMu.RUnlock()
	slices.Sort(names)
	return names
}

func NewFromConfig(cfg *Config) (Logger, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("logger: invalid config: %w", err)
	}
	factory, err := GetAdapter(cfg.Adapter.Type)
	if err != nil {
		return nil, err
	}
	opts := []Option{
		WithLevel(parseLevel(cfg.Core.Level)), WithName(cfg.Core.Name),
		WithFields(cfg.Core.Fields), WithEnableCaller(cfg.Core.EnableCaller),
		WithCallerSkipCount(cfg.Core.CallerSkip), WithEncoder(cfg.Output.Format),
		WithEnableStacktrace(cfg.Core.EnableStacktrace),
	}
	if cfg.Output.Console != nil {
		opts = append(opts, WithStdout(cfg.Output.Console.Enabled))
	}
	if file := cfg.Output.File; file != nil && file.Enabled {
		opts = append(opts, WithPath(file.Path), WithRotation(file.MaxSize, file.MaxAge, file.MaxBackups, file.Compress))
		opts = append(opts, func(o *Options) { o.LocalTime = file.LocalTime })
	}
	if plugin := cfg.Plugins.Sampling; plugin != nil && plugin.Enabled {
		excluded := make([]Level, 0, len(plugin.ExcludeLevels))
		for _, name := range plugin.ExcludeLevels {
			excluded = append(excluded, parseLevel(name))
		}
		opts = append(opts, WithSampling(SamplingConfig{
			Initial: plugin.Initial, Thereafter: plugin.Thereafter, Tick: plugin.Tick, ExcludeLevels: excluded,
		}))
	}
	if plugin := cfg.Plugins.Sanitize; plugin != nil && plugin.Enabled {
		sanitizer := SanitizerConfig{Enabled: true}
		matchers := make([]*regexp.Regexp, 0, len(plugin.Rules))
		rules := make([]SanitizerRule, 0, len(plugin.Rules))
		for _, configured := range plugin.Rules {
			pattern, err := regexp.Compile("(?i)" + configured.FieldPattern)
			if err != nil {
				return nil, fmt.Errorf("logger: sanitize pattern: %w", err)
			}
			rule := SanitizerRule{MaskChar: configured.MaskChar}
			switch configured.Strategy {
			case "mask_all":
				rule.Strategy = "mask"
			case "mask_partial":
				rule.Strategy = "mask"
				rule.KeepPrefix = 3
				rule.KeepSuffix = 4
			case "hash":
				rule.Strategy = "hash"
			}
			matchers = append(matchers, pattern)
			rules = append(rules, rule)
		}
		sanitizer.CustomMatcher = func(name string) *SanitizerRule {
			for i, matcher := range matchers {
				if matcher.MatchString(name) {
					return &rules[i]
				}
			}
			return nil
		}
		opts = append(opts, WithSanitizer(sanitizer))
	}
	if plugin := cfg.Plugins.Async; plugin != nil && plugin.Enabled {
		policy := "block"
		if plugin.DropOnFull {
			policy = "drop"
		}
		opts = append(opts, WithAsync(AsyncConfig{
			BufferSize: plugin.BufferSize, FlushInterval: plugin.FlushInterval, DropPolicy: policy,
		}))
	}
	log, err := factory(opts...)
	if err != nil {
		if log != nil {
			_ = closeLogger(log)
		}
		return nil, err
	}
	if log == nil {
		return nil, fmt.Errorf("logger: adapter %q returned no logger", cfg.Adapter.Type)
	}
	return log, nil
}

func parseLevel(level string) Level {
	if level == "panic" {
		return FatalLevel
	}
	parsed, _ := GetLevel(level)
	return parsed
}

// SetupLogger constructs and publishes a logger. The caller must close the
// returned root when it is no longer used; errors leave the default unchanged.
func SetupLogger(cfg *Config) (Logger, error) {
	log, err := NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	SetDefault(log)
	return log, nil
}
func NewDevelopmentLogger() (*Logrus, error) {
	return NewLogrusLogger(WithLevel(DebugLevel), WithStdout(true), WithEncoder("console"), WithEnableCaller(true))
}
func NewProductionLogger(path string) (*Logrus, error) {
	return NewLogrusLogger(
		WithLevel(InfoLevel), WithPath(path), WithEncoder("json"),
		WithSampling(SamplingConfig{Initial: 10, Thereafter: 100, Tick: time.Second}),
	)
}
func NewTestLogger() *Text { return NewLogger(WithLevel(DebugLevel)) }
