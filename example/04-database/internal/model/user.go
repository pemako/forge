package model

import "time"

// User is the GORM model — maps to the "users" table.
type User struct {
	ID        uint      `gorm:"primarykey"           json:"id"`
	Name      string    `gorm:"not null"             json:"name"`
	Email     string    `gorm:"uniqueIndex;not null" json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateUserRequest is the JSON body for POST /users.
// validate tags are enforced by validate.BindJSON.
type CreateUserRequest struct {
	Name  string `json:"name"  validate:"required,min=2,max=64"`
	Email string `json:"email" validate:"required,email"`
}

// UpdateUserRequest is the JSON body for PUT /users/:id.
// Both fields are optional: an empty value leaves the column unchanged.
type UpdateUserRequest struct {
	Name  string `json:"name"  validate:"omitempty,min=2,max=64"`
	Email string `json:"email" validate:"omitempty,email"`
}
