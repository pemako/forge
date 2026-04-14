package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pemako/forge/auth"
)

type testClaims struct {
	jwt.RegisteredClaims
	UserID int64 `json:"uid"`
}

const testSecret = "test-secret-key"

func TestGenerate_and_FromContext(t *testing.T) {
	claims := &testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID: 42,
	}
	token, err := auth.Generate(claims, auth.WithSecret(testSecret))
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Parse back
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.Required(
		auth.WithSecret(testSecret),
		auth.WithClaims(func() auth.Claims { return &testClaims{} }),
	))
	r.GET("/me", func(c *gin.Context) {
		got, ok := auth.FromContext[*testClaims](c.Request.Context())
		require.True(t, ok)
		assert.Equal(t, int64(42), got.UserID)
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestRequired_missingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// ErrorHandler must be registered before Required so its deferred code runs.
	r.Use(func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			c.JSON(401, gin.H{"error": c.Errors.Last().Error()})
		}
	})
	r.Use(auth.Required(auth.WithSecret(testSecret)))
	r.GET("/protected", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/protected", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestOptional_noToken_continues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.Optional(auth.WithSecret(testSecret)))
	r.GET("/public", func(c *gin.Context) {
		_, ok := auth.FromContext[*testClaims](c.Request.Context())
		assert.False(t, ok)
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/public", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}
