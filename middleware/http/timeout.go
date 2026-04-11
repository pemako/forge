package httpMiddleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		var once sync.Once
		timedOut := false

		done := make(chan struct{})
		go func() {
			c.Next()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			timedOut = true
			once.Do(func() {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable,
					errors.Define(503, "TIMEOUT", "request timeout"))
			})
		}
		if timedOut {
			<-done
		}
	}
}
