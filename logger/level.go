package logger

import (
	"context"
	"fmt"
	"os"
)

type Level int8

const (
	// TraceLevel level. Designates finer-grained informational events than the Debug.
	TraceLevel Level = iota - 2
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
	// InfoLevel is the default logging priority.
	// General operational entries about what's going on inside the application.
	InfoLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	ErrorLevel
	// FatalLevel level. Logs and then calls `logger.Exit(1)`. highest level of severity.
	FatalLevel
)

func (l Level) String() string {
	switch l {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	}
	return ""
}

// LevelForGorm 转换成gorm日志级别
func (l Level) LevelForGorm() int {
	switch l {
	case FatalLevel, ErrorLevel:
		return 2
	case WarnLevel:
		return 3
	case InfoLevel, DebugLevel, TraceLevel:
		return 4
	default:
		return 1
	}
}

// Enabled returns true if the given level is at or above this level.
func (l Level) Enabled(lvl Level) bool {
	return lvl >= l
}

// GetLevel converts a level string into a logger Level value.
// returns an error if the input string does not match known values.
func GetLevel(levelStr string) (Level, error) {
	switch levelStr {
	case TraceLevel.String():
		return TraceLevel, nil
	case DebugLevel.String():
		return DebugLevel, nil
	case InfoLevel.String():
		return InfoLevel, nil
	case WarnLevel.String():
		return WarnLevel, nil
	case ErrorLevel.String():
		return ErrorLevel, nil
	case FatalLevel.String():
		return FatalLevel, nil
	}
	return InfoLevel, fmt.Errorf("unknown level string %q, defaulting to InfoLevel", levelStr)
}

// Info logs a message at the info log level.
func Info(args ...any)                  { NewHelper(Default()).Info(args...) }
func Infof(format string, args ...any)  { Logf(InfoLevel, format, args...) }
func Trace(args ...any)                 { NewHelper(Default()).Trace(args...) }
func Tracef(format string, args ...any) { Logf(TraceLevel, format, args...) }
func Debug(args ...any)                 { NewHelper(Default()).Debug(args...) }
func Debugf(format string, args ...any) { Logf(DebugLevel, format, args...) }
func Warn(args ...any)                  { NewHelper(Default()).Warn(args...) }
func Warnf(format string, args ...any)  { Logf(WarnLevel, format, args...) }
func Error(args ...any)                 { NewHelper(Default()).Error(args...) }
func Errorf(format string, args ...any) { Logf(ErrorLevel, format, args...) }
func Fatal(args ...any) {
	Default().Log(context.Background(), FatalLevel, fmt.Sprint(args...))
	os.Exit(1)
}
func Fatalf(format string, args ...any) { Logf(FatalLevel, format, args...); os.Exit(1) }

func V(level Level, log Logger) bool {
	if log == nil {
		log = Default()
	}
	return log.Enabled(level)
}
