package httpMiddleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CORSConfig struct {
	AllowOrigins []string
	AllowMethods []string
	AllowHeaders []string
	MaxAge       time.Duration
}

func CORS(cfg CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		for _, o := range cfg.AllowOrigins {
			if o == "*" || o == origin {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
				c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
				if cfg.MaxAge > 0 {
					c.Header("Access-Control-Max-Age", fmt.Sprintf("%.0f", cfg.MaxAge.Seconds()))
				}
				break
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
