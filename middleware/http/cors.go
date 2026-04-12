package httpMiddleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds the Cross-Origin Resource Sharing policy for the CORS middleware.
type CORSConfig struct {
	// AllowOrigins is the list of permitted request origins. Use "*" to allow any origin.
	AllowOrigins []string
	// AllowMethods is the list of HTTP methods permitted in CORS requests (e.g., "GET", "POST").
	AllowMethods []string
	// AllowHeaders is the list of HTTP headers that can be used in CORS requests.
	AllowHeaders []string
	// MaxAge sets the Access-Control-Max-Age preflight cache duration.
	// Zero means the header is omitted.
	MaxAge time.Duration
}

// CORS returns a Gin middleware that applies the given CORS policy.
// Preflight OPTIONS requests are responded to immediately with HTTP 204.
// Non-matching origins receive no CORS headers (not blocked — the browser enforces).
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
