package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Plain gin handlers
// ---------------------------------------------------------------------------

func helloHandler(c *gin.Context) {
	name := c.DefaultQuery("name", "world")
	c.JSON(http.StatusOK, gin.H{"message": "Hello, " + name + "!"})
}

func pingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Huma handlers
// Input/output structs drive request validation, response serialization,
// and OpenAPI schema generation automatically.
// In a real service, handlers would call a service layer here.
// ---------------------------------------------------------------------------

func getUserHandler(_ context.Context, input *GetUserInput) (*GetUserOutput, error) {
	return &GetUserOutput{Body: &User{ID: input.ID, Name: "Alice"}}, nil
}

func createUserHandler(_ context.Context, input *CreateUserInput) (*CreateUserOutput, error) {
	return &CreateUserOutput{Body: &User{ID: 42, Name: input.Body.Name}}, nil
}
