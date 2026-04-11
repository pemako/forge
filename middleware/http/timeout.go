package httpMiddleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

// Timeout sets a deadline on the request context. Handlers that respect
// context cancellation will abort early. After c.Next() returns, if the
// context was cancelled due to our deadline, a 503 is written (only if
// nothing has been written yet).
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable,
				errors.Define(503, "TIMEOUT", "request timeout"))
		}
	}
}
