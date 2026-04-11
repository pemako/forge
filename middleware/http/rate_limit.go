package httpMiddleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
	"golang.org/x/time/rate"
)

type RateLimitConfig struct {
	Rate    float64
	Burst   int
	KeyFunc func(*gin.Context) string
}

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
