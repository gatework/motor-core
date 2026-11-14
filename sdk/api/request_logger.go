package api

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"
)

const requestIDLogField = "x-request-id"

// GetRequestLogger returns the request's supplied helper, or creates and caches
// one carrying its correlation ID. Initialize it before starting concurrent work.
func GetRequestLogger(c *gin.Context) *logger.Helper {
	if value, ok := c.Get(pkg.LoggerKey); ok {
		if log, ok := value.(*logger.Helper); ok && log != nil {
			return log
		}
	}
	log := logger.NewHelper(logger.Default().With(
		logger.FieldString(requestIDLogField, pkg.RequestID(c)),
	))
	c.Set(pkg.LoggerKey, log)
	return log
}

// SetRequestLogger ensures a request helper is available to later middleware.
func SetRequestLogger(c *gin.Context) {
	_ = GetRequestLogger(c)
}
