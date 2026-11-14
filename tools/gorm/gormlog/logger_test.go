package gormlog

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm/logger"

	logCore "github.com/gatework/motor-core/logger"
)

func TestNew(t *testing.T) {
	l := New(logger.Config{
		SlowThreshold: time.Second,
		Colorful:      true,
		LogLevel: logger.LogLevel(
			logCore.InfoLevel.LevelForGorm()),
	})
	l.Info(context.TODO(), "test")
}
