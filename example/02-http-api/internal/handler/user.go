package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/pemako/forge/errors"
	"github.com/pemako/forge/example/02-http-api/internal/model"
	"github.com/pemako/forge/example/02-http-api/internal/service"
	"github.com/pemako/forge/pagination"
	"github.com/pemako/forge/validate"
)

// pathID parses the :id path parameter, reporting 400 when it is not a
// positive integer.
func pathID(c *gin.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		return 0, errors.Wrap(errors.ErrBadRequest, "user id must be a positive integer")
	}
	return id, nil
}

func listUsersHandler(svc *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := pagination.FromQuery(c)
		users, total := svc.List(c.Request.Context(), pagination.Offset(p), p.PageSize)
		c.JSON(http.StatusOK, pagination.New(total, users, p))
	}
}

func getUserHandler(svc *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			c.Error(err)
			return
		}
		user, err := svc.Get(c.Request.Context(), id)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, user)
	}
}

func createUserHandler(svc *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateUserRequest
		if err := validate.BindJSON(c, &req); err != nil {
			c.Error(err)
			return
		}
		user, err := svc.Create(c.Request.Context(), req.Name, req.Email)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusCreated, user)
	}
}

func updateUserHandler(svc *service.UserService) gin.HandlerFunc {
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
		user, err := svc.Update(c.Request.Context(), id, req.Name, req.Email)
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, user)
	}
}

func deleteUserHandler(svc *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			c.Error(err)
			return
		}
		if err := svc.Delete(c.Request.Context(), id); err != nil {
			c.Error(err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
