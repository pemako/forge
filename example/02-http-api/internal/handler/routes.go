// Package handler wires all routes onto the gin engine.
// Auth: POST /auth/login is public; every /users route sits behind an
// auth.Required group, so handlers can read claims straight from the context.
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/pemako/forge/auth"
	"github.com/pemako/forge/core"
	"github.com/pemako/forge/example/02-http-api/internal/service"
)

// UserClaims is the JWT payload issued by POST /auth/login.
type UserClaims struct {
	jwt.RegisteredClaims
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
}

// Register mounts all routes on the gin engine.
//
// Do not add RequestID/Logger/Recovery/ErrorHandler here — server/http.New
// already mounts the default middleware stack.
func Register(r *gin.Engine, svc *service.UserService, cfg core.Config) {
	authOpts := []auth.Option{
		auth.WithConfig(cfg),
		auth.WithClaims(func() auth.Claims { return &UserClaims{} }),
	}

	// ── Auth (public) ─────────────────────────────────────────────────────────
	r.POST("/auth/login", loginHandler(authOpts))

	// ── Users (JWT required) ──────────────────────────────────────────────────
	// auth.Required rejects missing or invalid tokens with 401 before the
	// handler runs, and stores the parsed claims in the request context.
	users := r.Group("/users", auth.Required(authOpts...))
	users.GET("", listUsersHandler(svc))
	users.GET("/:id", getUserHandler(svc))
	users.POST("", createUserHandler(svc))
	users.PUT("/:id", updateUserHandler(svc))
	users.DELETE("/:id", deleteUserHandler(svc))
}
