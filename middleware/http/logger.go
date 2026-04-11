package httpMiddleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

func Logger(log core.Logger) gin.HandlerFunc {
	if log == nil {
		log = logger.Default()
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info(c.Request.Context(), "http request",
			core.Field{Key: "method", Value: c.Request.Method},
			core.Field{Key: "path", Value: c.FullPath()},
			core.Field{Key: "status", Value: c.Writer.Status()},
			core.Field{Key: "latency", Value: time.Since(start).String()},
			core.Field{Key: "request_id", Value: c.GetString("request_id")},
			core.Field{Key: "client_ip", Value: c.ClientIP()},
		)
	}
}
