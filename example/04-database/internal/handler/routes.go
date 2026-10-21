// Package handler wires the user CRUD routes onto the gin engine.
package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Register mounts all user CRUD routes on the gin engine.
//
// Do not add RequestID/Logger/Recovery/ErrorHandler here — server/http.New
// already mounts the default middleware stack.
func Register(r *gin.Engine, db *gorm.DB) {
	users := r.Group("/users")
	users.GET("", listUsersHandler(db))
	users.GET("/:id", getUserHandler(db))
	users.POST("", createUserHandler(db))
	users.PUT("/:id", updateUserHandler(db))
	users.DELETE("/:id", deleteUserHandler(db))
}
