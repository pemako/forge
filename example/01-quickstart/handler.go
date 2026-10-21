package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/validate"
)

// ---------------------------------------------------------------------------
// gin handlers
//
// Errors are reported with c.Error(err); the ErrorHandler middleware that
// server/http mounts by default converts them to a JSON response using the
// CodedError's HTTP status. Never write c.JSON(500, ...) by hand here.
// In a real service, handlers would call a service layer.
// ---------------------------------------------------------------------------

func helloHandler(c *gin.Context) {
	name := c.DefaultQuery("name", "world")
	c.JSON(http.StatusOK, gin.H{"message": "Hello, " + name + "!"})
}

func pingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func getUserHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		c.Error(errors.Wrap(errors.ErrBadRequest, "user id must be a positive integer"))
		return
	}
	c.JSON(http.StatusOK, &User{ID: id, Name: "Alice"})
}

func createUserHandler(c *gin.Context) {
	var req CreateUserRequest
	if err := validate.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, &User{ID: 42, Name: req.Name})
}
