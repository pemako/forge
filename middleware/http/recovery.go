package httpMiddleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

// Recovery returns a Gin middleware that catches panics and responds with
// HTTP 500 Internal Server Error. The panic value is not forwarded to the
// client; only the standard ErrInternal JSON body is returned.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, errors.ErrInternal)
	})
}
