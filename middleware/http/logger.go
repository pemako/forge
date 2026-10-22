package httpMiddleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
)

// Logger returns a Gin middleware that logs each completed request.
// The log entry includes method, path, HTTP status, latency, request_id, and
// client IP. If log is nil, the process-wide default logger is used.
//
// The level reflects the outcome: INFO below 400, WARN for 4xx, ERROR for 5xx,
// so a failing endpoint is visible without grepping for status codes.
//
// When the handler reported an error with c.Error, the entry also carries
// "error" and "error_reason". ErrorHandler strips the cause chain from the
// response body on purpose, which previously meant errors.Wrap context was
// written and then discarded entirely; this log line is where it survives.
func Logger(log core.Logger) gin.HandlerFunc {
	if log == nil {
		log = logger.Default()
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := []core.Field{
			{Key: "method", Value: c.Request.Method},
			{Key: "path", Value: c.FullPath()},
			{Key: "status", Value: status},
			{Key: "latency", Value: time.Since(start).String()},
			{Key: "request_id", Value: c.GetString("request_id")},
			{Key: "client_ip", Value: c.ClientIP()},
		}

		// ErrorHandler is mounted inside this middleware, so c.Errors is already
		// populated by the time c.Next returns.
		if ginErr := c.Errors.Last(); ginErr != nil && ginErr.Err != nil {
			// CodedError.Error() renders "message: cause", so this is where the
			// wrapped detail becomes visible.
			fields = append(fields, core.Field{Key: "error", Value: ginErr.Err.Error()})
			if ce, ok := errors.AsType[*errors.CodedError](ginErr.Err); ok {
				fields = append(fields, core.Field{Key: "error_reason", Value: ce.Reason})
			}
		}

		ctx := c.Request.Context()
		switch {
		case status >= 500:
			log.Error(ctx, "http request", fields...)
		case status >= 400:
			log.Warn(ctx, "http request", fields...)
		default:
			log.Info(ctx, "http request", fields...)
		}
	}
}
