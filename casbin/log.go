package mycasbin

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/gatework/motor-core/logger"
)

// Logger is the implementation for a Logger using golang log.
type Logger struct {
	enable int32
}

// EnableLog controls whether print the message.
func (l *Logger) EnableLog(enable bool) {
	i := 0
	if enable {
		i = 1
	}
	atomic.StoreInt32(&(l.enable), int32(i))
}

// IsEnabled returns if logger is enabled.
func (l *Logger) IsEnabled() bool {
	return atomic.LoadInt32(&(l.enable)) != 0
}

// LogModel log info related to model.
func (l *Logger) LogModel(model [][]string) {
	if !l.IsEnabled() || !logger.Default().Enabled(logger.InfoLevel) {
		return
	}
	var message strings.Builder
	for _, row := range model {
		for _, value := range row {
			message.WriteByte(' ')
			message.WriteString(value)
		}
		message.WriteByte('\n')
	}
	logger.Default().Log(context.Background(), logger.InfoLevel, message.String())
}

// LogEnforce records a policy decision when enforcement logging is enabled.
func (l *Logger) LogEnforce(matcher string, request []any, result bool, explains [][]string) {
	if !l.IsEnabled() {
		return
	}
	logger.Default().Log(
		context.Background(), logger.InfoLevel, "",
		logger.FieldString("matcher", matcher), logger.Any("request", request),
		logger.Bool("result", result), logger.Any("explains", explains),
	)
}

// LogRole records a role relationship when enforcement logging is enabled.
func (l *Logger) LogRole(roles []string) {
	if !l.IsEnabled() {
		return
	}
	logger.Default().Log(context.Background(), logger.InfoLevel, "", logger.Any("roles", roles))
}

// LogPolicy records configured policies when enforcement logging is enabled.
func (l *Logger) LogPolicy(policy map[string][][]string) {
	if !l.IsEnabled() {
		return
	}
	fields := make([]logger.Field, 0, len(policy))
	for key, value := range policy {
		fields = append(fields, logger.Any(key, value))
	}
	logger.Default().Log(context.Background(), logger.InfoLevel, "", fields...)
}
