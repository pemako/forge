package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/pemako/forge/auth"
	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/example/02-http-api/internal/model"
	"github.com/pemako/forge/validate"
)

func loginHandler(authOpts []auth.Option) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.LoginRequest
		if err := validate.BindJSON(c, &req); err != nil {
			c.Error(err)
			return
		}

		// Demo: accept any well-formed email + password.
		// In production: look up user by email and verify hashed password.
		claims := &UserClaims{
			RegisteredClaims: jwt.RegisteredClaims{},
			UserID:           1,
			Email:            req.Email,
		}
		token, err := auth.Generate(claims, authOpts...)
		if err != nil {
			c.Error(errors.Wrap(errors.ErrInternal, "generate token: %v", err))
			return
		}
		c.JSON(http.StatusOK, model.LoginResponse{Token: token})
	}
}
