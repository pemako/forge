package main

// User is the shared domain type returned by user handlers.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CreateUserRequest is the JSON request body for POST /users.
// Tags are enforced by the validate package via validate.BindJSON.
type CreateUserRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}
