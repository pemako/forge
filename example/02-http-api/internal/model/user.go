package model

import (
	"time"

	"github.com/pemako/forge/pagination"
)

// User is the core domain entity.
type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ── Auth ─────────────────────────────────────────────────────────────────────

type LoginInput struct {
	Body struct {
		Email    string `json:"email"    doc:"Email address" format:"email"`
		Password string `json:"password" doc:"Password"      minLength:"6"`
	}
}

type LoginOutput struct {
	Body struct {
		Token string `json:"token" doc:"JWT access token"`
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

type ListUsersInput struct {
	Page     int `query:"page"      doc:"Page number (1-based), defaults to 1"     minimum:"1"`
	PageSize int `query:"page_size" doc:"Items per page, defaults to 20, max 100"  minimum:"1" maximum:"100"`
}

type ListUsersOutput struct {
	Body *pagination.Result[*User]
}

// ── Get ──────────────────────────────────────────────────────────────────────

type GetUserInput struct {
	ID int `path:"id" doc:"User ID" minimum:"1"`
}

type GetUserOutput struct {
	Body *User
}

// ── Create ───────────────────────────────────────────────────────────────────

type CreateUserInput struct {
	Body struct {
		Name  string `json:"name"  doc:"Display name"  minLength:"2" maxLength:"64"`
		Email string `json:"email" doc:"Email address" format:"email"`
	}
}

type CreateUserOutput struct {
	Body *User
}

// ── Update ───────────────────────────────────────────────────────────────────

type UpdateUserInput struct {
	ID   int `path:"id" doc:"User ID" minimum:"1"`
	Body struct {
		Name  string `json:"name"  doc:"Display name"  minLength:"2" maxLength:"64"`
		Email string `json:"email" doc:"Email address" format:"email"`
	}
}

type UpdateUserOutput struct {
	Body *User
}

// ── Delete ───────────────────────────────────────────────────────────────────

type DeleteUserInput struct {
	ID int `path:"id" doc:"User ID" minimum:"1"`
}

type DeleteUserOutput struct{}
