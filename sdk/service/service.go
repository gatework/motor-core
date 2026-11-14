// Package service defines the dependencies shared by application services.
package service

import (
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/storage"
	"gorm.io/gorm"
)

// Service carries dependencies, not request context or accumulated errors.
// Operations implemented by the host take context.Context as their first input.
type Service struct {
	DB     *gorm.DB
	Logger *logger.Helper
	Cache  storage.Cache
}
