package config

import (
	"fmt"
	"os"

	"github.com/gatework/motor-core/logger"
)

// RotationConfig controls retention and rotation for owned log files.
type RotationConfig struct {
	MaxSize    int  `yaml:"maxSize" json:"maxSize"`
	MaxAge     int  `yaml:"maxAge" json:"maxAge"`
	MaxBackups int  `yaml:"maxBackups" json:"maxBackups"`
	Compress   bool `yaml:"compress" json:"compress"`
}

// Logger describes an explicitly constructed application logger.
type Logger struct {
	Type         string          `yaml:"type" json:"type"`
	Adapter      string          `yaml:"adapter" json:"adapter"`
	Path         string          `yaml:"path" json:"path"`
	Level        string          `yaml:"level" json:"level"`
	Stdout       string          `yaml:"stdout" json:"stdout"`
	Encoder      string          `yaml:"encoder" json:"encoder"`
	EnableCaller bool            `yaml:"enableCaller" json:"enableCaller"`
	EnabledDB    bool            `yaml:"enableddb" json:"enableddb"`
	Cap          uint            `yaml:"cap" json:"cap"`
	Rotation     *RotationConfig `yaml:"rotation" json:"rotation"`
}

// Open creates a logger owned by the caller and reports output failures.
func (l Logger) Open() (logger.Logger, error) {
	if err := l.validate(); err != nil {
		return nil, err
	}
	if l.Path != "" {
		if err := os.MkdirAll(l.Path, 0o750); err != nil {
			return nil, fmt.Errorf("config: create log directory: %w", err)
		}
	}
	opts := []logger.Option{
		logger.WithName("motor"), logger.WithCallerSkipCount(2),
		logger.WithStdout(l.Stdout != "file" && l.Stdout != ""),
		logger.WithEnableCaller(l.EnableCaller),
	}
	if l.Level != "" {
		level, err := logger.GetLevel(l.Level)
		if err != nil {
			return nil, err
		}
		opts = append(opts, logger.WithLevel(level))
	}
	if l.Path != "" {
		opts = append(opts, logger.WithPath(l.Path))
	}
	if l.Encoder != "" {
		opts = append(opts, logger.WithEncoder(l.Encoder))
	}
	if l.Path != "" && l.Rotation != nil {
		opts = append(opts, logger.WithRotation(
			l.Rotation.MaxSize,
			l.Rotation.MaxAge,
			l.Rotation.MaxBackups,
			l.Rotation.Compress,
		))
	}
	switch l.adapterType() {
	case "zap":
		return logger.NewZapLogger(opts...)
	case "", "logrus":
		return logger.NewLogrusLogger(opts...)
	default:
		return nil, fmt.Errorf("config: unsupported logger adapter %q", l.adapterType())
	}
}

func (l Logger) adapterType() string {
	if l.Adapter != "" {
		return l.Adapter
	}
	return l.Type
}

// validate checks settings without opening outputs or changing a process logger.
func (l Logger) validate() error {
	switch l.adapterType() {
	case "", "logrus", "zap":
	default:
		return fmt.Errorf("config: unsupported logger adapter %q", l.adapterType())
	}
	if l.Level != "" {
		if _, err := logger.GetLevel(l.Level); err != nil {
			return err
		}
	}
	switch l.Encoder {
	case "", "json", "text", "console":
	default:
		return fmt.Errorf("config: unsupported logger encoder %q", l.Encoder)
	}
	if r := l.Rotation; r != nil && (r.MaxSize < 0 || r.MaxAge < 0 || r.MaxBackups < 0) {
		return fmt.Errorf("config: logger rotation values cannot be negative")
	}
	return nil
}
