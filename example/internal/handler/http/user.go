package httphandler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pemako/mk/auth"
	mkerrors "github.com/pemako/mk/errors"
	"github.com/pemako/mk/pagination"
	"github.com/pemako/mk/validate"

	"github.com/pemako/mk/example/internal/model"
	"github.com/pemako/mk/example/internal/service"
)

// UserHandler exposes HTTP endpoints for the user domain.
// It demonstrates validate (BindJSON), pagination (FromQuery/New/Offset),
// auth (Generate/FromContext), and the full service layer.
type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// RegisterRoutes mounts all user-domain routes on r.
// The /api/v1/login route is registered separately in main.go so that it
// appears at the top-level alongside the protected group.
func (h *UserHandler) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/api/v1/users")
	g.POST("", h.Create)
	g.POST("/locked", h.CreateWithLock)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.GET("/:id/cached", h.GetCached)

	posts := r.Group("/api/v1/posts")
	posts.GET("/:id", h.GetExternalPost)
}

// Create handles POST /api/v1/users.
// Demonstrates validate.BindJSON for request binding and validation.
func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Name  string `json:"name"  validate:"required,min=2,max=64"`
		Email string `json:"email" validate:"required,email"`
	}
	if err := validate.BindJSON(c, &req); err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	u, err := h.svc.Create(c.Request.Context(), req.Name, req.Email)
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusCreated, u)
}

// Get handles GET /api/v1/users/:id.
func (h *UserHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	u, err := h.svc.Get(c.Request.Context(), uint(id))
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, u)
}

// List handles GET /api/v1/users.
// Demonstrates pagination.FromQuery, pagination.Offset, and pagination.New.
func (h *UserHandler) List(c *gin.Context) {
	p := pagination.FromQuery(c)
	users, err := h.svc.List(c.Request.Context(), pagination.Offset(p), p.PageSize)
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	total, err := h.svc.Count(c.Request.Context())
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, pagination.New(total, users, p))
}

// GetCached handles GET /api/v1/users/:id/cached.
// Demonstrates the cache-aside pattern via the service layer.
func (h *UserHandler) GetCached(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	u, err := h.svc.GetCached(c.Request.Context(), uint(id))
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, u)
}

// GetExternalPost handles GET /api/v1/posts/:id.
// Proxies to JSONPlaceholder, demonstrating httpclient + circuitbreaker.
func (h *UserHandler) GetExternalPost(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	post, err := h.svc.GetExternalPost(c.Request.Context(), id)
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, post)
}

// CreateWithLock handles POST /api/v1/users/locked.
// Demonstrates the distributed lock pattern via the service layer.
func (h *UserHandler) CreateWithLock(c *gin.Context) {
	var req struct {
		Name  string `json:"name"  validate:"required,min=2,max=64"`
		Email string `json:"email" validate:"required,email"`
	}
	if err := validate.BindJSON(c, &req); err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	u, err := h.svc.CreateWithLock(c.Request.Context(), req.Name, req.Email)
	if err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	c.JSON(http.StatusCreated, u)
}

// Login handles POST /api/v1/login.
// Accepts any valid email+password and returns a signed JWT (demo — no
// real credential check). Demonstrates auth.Generate.
func (h *UserHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email"    validate:"required,email"`
		Password string `json:"password" validate:"required,min=6"`
	}
	if err := validate.BindJSON(c, &req); err != nil {
		c.Error(err) //nolint:errcheck
		return
	}
	token, err := auth.Generate(&model.UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
		UserID: 1,
		Role:   "user",
	}, auth.WithSecret("example-jwt-secret-change-in-production"))
	if err != nil {
		c.Error(mkerrors.ErrInternal) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// GetMe handles GET /api/v1/me (protected route).
// Demonstrates auth.FromContext[T] to extract typed JWT claims.
func (h *UserHandler) GetMe(c *gin.Context) {
	claims, ok := auth.FromContext[*model.UserClaims](c.Request.Context())
	if !ok {
		c.Error(mkerrors.ErrUnAuth) //nolint:errcheck
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": claims.UserID, "role": claims.Role})
}
