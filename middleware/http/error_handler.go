package httpMiddleware

import (
	"github.com/gin-gonic/gin"
	"github.com/pemako/forge/errors"
)

// ErrorHandler translates gin.Context errors to JSON responses.
// *errors.CodedError -> its code + body.
// Other errors -> 500 Internal (message not forwarded).
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err
		var ce *errors.CodedError
		if errors.As(err, &ce) {
			c.JSON(ce.Code, ce)
		} else {
			c.JSON(500, errors.ErrInternal)
		}
	}
}
