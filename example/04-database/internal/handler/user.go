package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	forgerrors "github.com/pemako/forge/errors"
	"github.com/pemako/forge/example/04-database/internal/model"
	"github.com/pemako/forge/pagination"
	"github.com/pemako/forge/validate"
)

// pathID parses the :id path parameter, reporting 400 when it is not a
// positive integer.
func pathID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, forgerrors.Wrap(forgerrors.ErrBadRequest, "user id must be a positive integer")
	}
	return uint(id), nil
}

func listUsersHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		p := pagination.FromQuery(c)

		var total int64
		if err := db.WithContext(ctx).Model(&model.User{}).Count(&total).Error; err != nil {
			c.Error(forgerrors.Wrap(forgerrors.ErrInternal, "count users: %v", err))
			return
		}

		var users []*model.User
		if err := db.WithContext(ctx).
			Offset(pagination.Offset(p)).Limit(p.PageSize).
			Find(&users).Error; err != nil {
			c.Error(forgerrors.Wrap(forgerrors.ErrInternal, "list users: %v", err))
			return
		}

		c.JSON(http.StatusOK, pagination.New(total, users, p))
	}
}

func getUserHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			c.Error(err)
			return
		}
		var user model.User
		err = db.WithContext(c.Request.Context()).First(&user, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(forgerrors.Wrap(forgerrors.ErrNotFound, "user %d not found", id))
			return
		}
		if err != nil {
			c.Error(forgerrors.Wrap(forgerrors.ErrInternal, "get user: %v", err))
			return
		}
		c.JSON(http.StatusOK, &user)
	}
}

func createUserHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateUserRequest
		if err := validate.BindJSON(c, &req); err != nil {
			c.Error(err)
			return
		}
		user := &model.User{Name: req.Name, Email: req.Email}
		if err := db.WithContext(c.Request.Context()).Create(user).Error; err != nil {
			// SQLite unique constraint violation
			c.Error(forgerrors.Wrap(forgerrors.ErrConflict, "email already in use: %v", err))
			return
		}
		c.JSON(http.StatusCreated, user)
	}
}

func updateUserHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			c.Error(err)
			return
		}
		var req model.UpdateUserRequest
		if err := validate.BindJSON(c, &req); err != nil {
			c.Error(err)
			return
		}

		ctx := c.Request.Context()
		var user model.User
		err = db.WithContext(ctx).First(&user, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.Error(forgerrors.Wrap(forgerrors.ErrNotFound, "user %d not found", id))
			return
		}
		if err != nil {
			c.Error(forgerrors.Wrap(forgerrors.ErrInternal, "get user: %v", err))
			return
		}

		if req.Name != "" {
			user.Name = req.Name
		}
		if req.Email != "" {
			user.Email = req.Email
		}
		if err := db.WithContext(ctx).Save(&user).Error; err != nil {
			c.Error(forgerrors.Wrap(forgerrors.ErrInternal, "update user: %v", err))
			return
		}
		c.JSON(http.StatusOK, &user)
	}
}

func deleteUserHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			c.Error(err)
			return
		}
		result := db.WithContext(c.Request.Context()).Delete(&model.User{}, id)
		if result.Error != nil {
			c.Error(forgerrors.Wrap(forgerrors.ErrInternal, "delete user: %v", result.Error))
			return
		}
		if result.RowsAffected == 0 {
			c.Error(forgerrors.Wrap(forgerrors.ErrNotFound, "user %d not found", id))
			return
		}
		c.Status(http.StatusNoContent)
	}
}
