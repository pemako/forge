package main

// User is the shared domain type returned by user handlers.
type User struct {
	ID   int    `json:"id"   doc:"Unique user ID"`
	Name string `json:"name" doc:"Display name"`
}

// GetUserInput carries the path parameter for GET /users/{id}.
type GetUserInput struct {
	ID int `path:"id" doc:"User ID" minimum:"1"`
}

// GetUserOutput wraps the response body for GET /users/{id}.
type GetUserOutput struct {
	Body *User
}

// CreateUserInput carries the JSON request body for POST /users.
type CreateUserInput struct {
	Body struct {
		Name string `json:"name" doc:"Display name" minLength:"1" maxLength:"100"`
	}
}

// CreateUserOutput wraps the 201 response body for POST /users.
type CreateUserOutput struct {
	Body *User
}
