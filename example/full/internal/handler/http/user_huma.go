package httphandler

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/golang-jwt/jwt/v5"
	mkerrors "github.com/pemako/mk/errors"
	"github.com/pemako/mk/pagination"

	"github.com/pemako/mk/example/full/internal/model"
	"github.com/pemako/mk/example/full/internal/service"
)

// RegisterHumaRoutes registers all user-domain routes on the Huma API.
// Calling this replaces the gin-style RegisterRoutes for documentation purposes.
func RegisterHumaRoutes(api huma.API, svc *service.UserService, jwtSecret string) {
	h := &humaUserHandler{svc: svc, jwtSecret: jwtSecret}

	huma.Register(api, huma.Operation{
		Method:        "POST",
		Path:          "/api/v1/users",
		Summary:       "Create user",
		Tags:          []string{"users"},
		DefaultStatus: 201,
	}, h.create)

	huma.Register(api, huma.Operation{
		Method:  "GET",
		Path:    "/api/v1/users/{id}",
		Summary: "Get user by ID",
		Tags:    []string{"users"},
	}, h.get)

	huma.Register(api, huma.Operation{
		Method:  "GET",
		Path:    "/api/v1/users",
		Summary: "List users (paginated)",
		Tags:    []string{"users"},
	}, h.list)

	huma.Register(api, huma.Operation{
		Method:      "POST",
		Path:        "/api/v1/login",
		Summary:     "Login",
		Description: "Returns a signed JWT. Demo only — no real credential check.",
		Tags:        []string{"auth"},
	}, h.login)

	huma.Register(api, huma.Operation{
		Method:      "GET",
		Path:        "/api/v1/me",
		Summary:     "Get current user",
		Description: "Requires Authorization: Bearer <token> header.",
		Tags:        []string{"auth"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, h.me)
}

// --- types ---

type humaUserHandler struct {
	svc       *service.UserService
	jwtSecret string
}

type createUserInput struct {
	Body struct {
		Name  string `json:"name"  minLength:"2" maxLength:"64" doc:"Display name"`
		Email string `json:"email" format:"email"               doc:"Contact email"`
	}
}

type userOutput struct {
	Body *model.User
}

type listUsersInput struct {
	Page     int `query:"page"      default:"1"  minimum:"1"   doc:"Page number"`
	PageSize int `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"Items per page"`
}

type listUsersOutput struct {
	Body pagination.Result[model.User]
}

type getUserInput struct {
	ID uint `path:"id" doc:"User ID"`
}

type loginInput struct {
	Body struct {
		Email    string `json:"email"    format:"email" doc:"User email"`
		Password string `json:"password" minLength:"6"  doc:"Password (demo: any value ≥6 chars)"`
	}
}

type loginOutput struct {
	Body struct {
		Token string `json:"token" doc:"Signed JWT bearer token"`
	}
}

type meInput struct {
	Authorization string `header:"Authorization" doc:"Bearer <token>"`
}

type meOutput struct {
	Body struct {
		UserID int64  `json:"user_id"`
		Role   string `json:"role"`
	}
}

// --- handlers ---

func (h *humaUserHandler) create(ctx context.Context, input *createUserInput) (*userOutput, error) {
	u, err := h.svc.Create(ctx, input.Body.Name, input.Body.Email)
	if err != nil {
		return nil, err
	}
	return &userOutput{Body: u}, nil
}

func (h *humaUserHandler) get(ctx context.Context, input *getUserInput) (*userOutput, error) {
	u, err := h.svc.Get(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	return &userOutput{Body: u}, nil
}

func (h *humaUserHandler) list(ctx context.Context, input *listUsersInput) (*listUsersOutput, error) {
	offset := (input.Page - 1) * input.PageSize
	users, err := h.svc.List(ctx, offset, input.PageSize)
	if err != nil {
		return nil, err
	}
	total, err := h.svc.Count(ctx)
	if err != nil {
		return nil, err
	}
	p := pagination.Params{Page: input.Page, PageSize: input.PageSize}
	return &listUsersOutput{Body: pagination.New(total, users, p)}, nil
}

func (h *humaUserHandler) login(_ context.Context, input *loginInput) (*loginOutput, error) {
	claims := &model.UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
		UserID: 1,
		Role:   "user",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.jwtSecret))
	if err != nil {
		return nil, mkerrors.ErrInternal
	}
	out := &loginOutput{}
	out.Body.Token = token
	return out, nil
}

func (h *humaUserHandler) me(_ context.Context, input *meInput) (*meOutput, error) {
	if input.Authorization == "" {
		return nil, huma.Error401Unauthorized("missing Authorization header", mkerrors.ErrUnauthorized)
	}
	raw := strings.TrimPrefix(input.Authorization, "Bearer ")
	claims := &model.UserClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) {
		return []byte(h.jwtSecret), nil
	})
	if err != nil {
		return nil, huma.Error401Unauthorized("invalid token", mkerrors.ErrUnauthorized)
	}
	out := &meOutput{}
	out.Body.UserID = claims.UserID
	out.Body.Role = claims.Role
	return out, nil
}
