package httpMiddleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

// BodyLimit returns a Gin middleware that enforces a maximum request body size.
// size is a human-readable string like "8MB", "1GB", or "512KB".
// When the limit is exceeded, the middleware responds with HTTP 413
// Payload Too Large. If size cannot be parsed, the default of 8 MB is used.
func BodyLimit(size string) gin.HandlerFunc {
	maxBytes := parseSize(size)
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
		for _, e := range c.Errors {
			if e.Err != nil && strings.Contains(e.Err.Error(), "http: request body too large") {
				c.AbortWithStatusJSON(413, errors.Define(413, "PAYLOAD_TOO_LARGE", "request body too large"))
				return
			}
		}
	}
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	units := map[string]int64{"GB": 1 << 30, "MB": 1 << 20, "KB": 1 << 10, "B": 1}
	for suffix, mul := range units {
		if strings.HasSuffix(s, suffix) {
			var n int64
			fmt.Sscanf(strings.TrimSuffix(s, suffix), "%d", &n)
			return n * mul
		}
	}
	return 8 << 20
}
