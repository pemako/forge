package httpMiddleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

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
