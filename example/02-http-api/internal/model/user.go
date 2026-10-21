package model

import "time"

// User is the core domain entity.
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ── Auth ─────────────────────────────────────────────────────────────────────

// LoginRequest is the JSON body for POST /auth/login.
// validate tags are enforced by validate.BindJSON.
type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

// LoginResponse carries the issued JWT.
type LoginResponse struct {
	Token string `json:"token"`
}

// ── Users ────────────────────────────────────────────────────────────────────

// CreateUserRequest is the JSON body for POST /users.
type CreateUserRequest struct {
	Name  string `json:"name"  validate:"required,min=2,max=64"`
	Email string `json:"email" validate:"required,email"`
}

// UpdateUserRequest is the JSON body for PUT /users/:id.
type UpdateUserRequest struct {
	Name  string `json:"name"  validate:"required,min=2,max=64"`
	Email string `json:"email" validate:"required,email"`
}
