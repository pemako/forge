package auth

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/pemako/mk/core"
	mkerrors "github.com/pemako/mk/errors"
)

type contextKey struct{}

// Generate signs and returns a JWT token string for the given claims using HS256.
// The claims struct must embed jwt.RegisteredClaims.
func Generate(claims Claims, opts ...Option) (string, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if o.secret == nil {
		return "", mkerrors.Wrap(mkerrors.ErrInternal, "auth: secret is required")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(o.secret)
}

// FromContext extracts typed claims stored by Required or Optional middleware.
// Returns the zero value and false if no claims are present in ctx.
//
//	claims, ok := auth.FromContext[*UserClaims](c.Request.Context())
func FromContext[T Claims](ctx context.Context) (T, bool) {
	v := ctx.Value(contextKey{})
	if v == nil {
		var zero T
		return zero, false
	}
	claims, ok := v.(T)
	return claims, ok
}

// Required returns a gin middleware that validates the Bearer token in the
// Authorization header. Requests without a valid token receive 401.
// Parsed claims are stored in the request context; retrieve with FromContext.
func Required(opts ...Option) gin.HandlerFunc {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr == "" {
			_ = c.Error(mkerrors.Define(401, "MISSING_TOKEN", "authorization token required"))
			c.Abort()
			return
		}
		claims, err := parseToken(tokenStr, o)
		if err != nil {
			o.log.Debug(c.Request.Context(), "auth: invalid token",
				core.Field{Key: "err", Value: err.Error()})
			_ = c.Error(mkerrors.Define(401, "INVALID_TOKEN", "invalid or expired token"))
			c.Abort()
			return
		}
		ctx := context.WithValue(c.Request.Context(), contextKey{}, claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// Optional returns a gin middleware that parses the Bearer token if present.
// If the token is absent or invalid the request continues without claims.
func Optional(opts ...Option) gin.HandlerFunc {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr != "" {
			if claims, err := parseToken(tokenStr, o); err == nil {
				ctx := context.WithValue(c.Request.Context(), contextKey{}, claims)
				c.Request = c.Request.WithContext(ctx)
			}
		}
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return c.Query("token")
}

func parseToken(tokenStr string, o options) (Claims, error) {
	if o.claimsFunc != nil {
		claims := o.claimsFunc()
		_, err := jwt.ParseWithClaims(tokenStr, claims, o.keyFunc)
		if err != nil {
			return nil, err
		}
		return claims, nil
	}
	token, err := jwt.Parse(tokenStr, o.keyFunc)
	if err != nil {
		return nil, err
	}
	if mc, ok := token.Claims.(jwt.MapClaims); ok {
		return mc, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}
