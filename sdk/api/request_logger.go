package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"
)

// GetRequestLogger 获取上下文提供的日志
func GetRequestLogger(c *gin.Context) *logger.Helper {
	var log *logger.Helper
	l, ok := c.Get(pkg.LoggerKey)
	if ok {
		ok = false
		log, ok = l.(*logger.Helper)
		if ok {
			return log
		}
	}
	//如果没有在上下文中放入logger
	requestId := pkg.RequestID(c)
	log = logger.NewHelper(logger.Default()).WithFields(map[string]any{
		strings.ToLower(pkg.TrafficKey): requestId,
	})
	return log
}

// SetRequestLogger 设置logger中间件
func SetRequestLogger(c *gin.Context) {
	requestId := pkg.RequestID(c)
	log := logger.NewHelper(logger.Default()).WithFields(map[string]any{
		strings.ToLower(pkg.TrafficKey): requestId,
	})
	c.Set(pkg.LoggerKey, log)
}
