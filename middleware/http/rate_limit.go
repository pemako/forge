package httpMiddleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
	"golang.org/x/time/rate"
)

// RateLimitConfig configures the token-bucket rate limiter middleware.
type RateLimitConfig struct {
	// Rate is the number of requests per second allowed for each key.
	Rate float64
	// Burst is the maximum burst size above the sustained rate.
	Burst int
	// KeyFunc extracts the rate limit key from a request.
	// Defaults to c.ClientIP() when nil.
	KeyFunc func(*gin.Context) string
}

// RateLimit returns a Gin middleware that enforces a per-key token-bucket rate
// limit. A separate limiter is maintained for each unique key returned by
// KeyFunc. Requests that exceed the limit receive HTTP 429 Too Many Requests.
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *gin.Context) string { return c.ClientIP() }
	}
	var mu sync.Map
	getLimiter := func(key string) *rate.Limiter {
		v, _ := mu.LoadOrStore(key, rate.NewLimiter(rate.Limit(cfg.Rate), cfg.Burst))
		return v.(*rate.Limiter)
	}
	return func(c *gin.Context) {
		if !getLimiter(cfg.KeyFunc(c)).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, errors.ErrTooMany)
			return
		}
		c.Next()
	}
}
