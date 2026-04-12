// Package httpMiddleware provides Gin middleware for mk HTTP servers.
// The middleware in this package is pre-mounted by server/http.New in the
// standard order: RequestID, Tracing, Logger, Metrics, Recovery, ErrorHandler.
// Additional middleware (CORS, Timeout, BodyLimit, RateLimit, IPFilter) must be
// mounted explicitly via Engine().Use().
package httpMiddleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderRequestID is the HTTP header name used to propagate request identifiers.
const HeaderRequestID = "X-Request-ID"

// RequestID is a Gin middleware that ensures every request has a unique
// identifier. If the incoming request already carries an X-Request-ID header,
// that value is reused; otherwise a new UUID is generated. The identifier is
// stored in the Gin context under the key "request_id" and echoed back in the
// response via the X-Request-ID header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}
