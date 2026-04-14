// Package handler wires all routes onto the huma API.
// All routes use huma.Register so they appear in Swagger UI.
// Auth: login is public; all other routes call requireAuth which
// checks JWT claims stored in context by auth.Optional global middleware.
package handler

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
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

// Register mounts all routes on the huma API.
func Register(r *gin.Engine, api huma.API, svc *service.UserService, cfg core.Config) {
	authOpts := []auth.Option{
		auth.WithConfig(cfg),
		auth.WithClaims(func() auth.Claims { return &UserClaims{} }),
	}

	// Register bearerAuth security scheme so Swagger UI shows the
	// "Authorize" button and lets users enter their JWT token.
	oapi := api.OpenAPI()
	if oapi.Components == nil {
		oapi.Components = &huma.Components{}
	}
	oapi.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		},
	}
	// Apply bearerAuth globally — all operations require auth by default.
	// Public operations override this with Security: []map[string][]string{}.
	oapi.Security = []map[string][]string{{"bearerAuth": {}}}

	// auth.Optional parses the Bearer token if present and stores claims in
	// context without blocking. Protected handlers call requireAuth themselves.
	r.Use(auth.Optional(authOpts...))

	// ── Auth ──────────────────────────────────────────────────────────────────
	huma.Register(api, huma.Operation{
		Method:      http.MethodPost,
		Path:        "/auth/login",
		Summary:     "Login",
		Description: "Returns a JWT. Pass it as: Authorization: Bearer <token>",
		Tags:        []string{"auth"},
		Security:    []map[string][]string{}, // public: override global bearer requirement
	}, loginHandler(authOpts))

	// ── Users ─────────────────────────────────────────────────────────────────
	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/users",
		Summary: "List users",
		Tags:    []string{"users"},
	}, listUsersHandler(svc))

	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/users/{id}",
		Summary: "Get user",
		Tags:    []string{"users"},
	}, getUserHandler(svc))

	huma.Register(api, huma.Operation{
		Method:        http.MethodPost,
		Path:          "/users",
		Summary:       "Create user",
		DefaultStatus: http.StatusCreated,
		Tags:          []string{"users"},
	}, createUserHandler(svc))

	huma.Register(api, huma.Operation{
		Method:  http.MethodPut,
		Path:    "/users/{id}",
		Summary: "Update user",
		Tags:    []string{"users"},
	}, updateUserHandler(svc))

	huma.Register(api, huma.Operation{
		Method:        http.MethodDelete,
		Path:          "/users/{id}",
		Summary:       "Delete user",
		DefaultStatus: http.StatusNoContent,
		Tags:          []string{"users"},
	}, deleteUserHandler(svc))
}
