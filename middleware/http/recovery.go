package httpMiddleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/errors"
)

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, errors.ErrInternal)
	})
}
