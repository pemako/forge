// Package validate provides request binding and struct validation helpers
// for gin handlers, backed by go-playground/validator/v10.
// All functions return a 400 CodedError with field-level details on failure.
package validate

import (
	"errors"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	mkerrors "github.com/pemako/mk/errors"
)

var (
	once     sync.Once
	instance *validator.Validate
)

func getInstance() *validator.Validate {
	once.Do(func() {
		instance = validator.New()
	})
	return instance
}

// Register adds a custom validation function for the given tag.
// Must be called before the first use of any Bind* or Struct function.
func Register(tag string, fn validator.Func) error {
	return getInstance().RegisterValidation(tag, fn)
}

// Struct validates a struct using its `validate` tags.
// Returns a 400 CodedError with field-level details on failure.
// Can be used outside gin handlers (e.g., in the service layer).
func Struct(obj any) error {
	if err := getInstance().Struct(obj); err != nil {
		return toCodedError(err)
	}
	return nil
}

// Bind binds the request body based on Content-Type and validates the result.
// Supports JSON (application/json) and form data (application/x-www-form-urlencoded,
// multipart/form-data). Use BindJSON or BindForm to bind explicitly.
func Bind(c *gin.Context, obj any) error {
	if err := c.ShouldBind(obj); err != nil {
		return mkerrors.Define(400, "BAD_REQUEST", err.Error())
	}
	return Struct(obj)
}

// BindJSON binds the JSON request body into obj and validates it.
// Use struct tags: `json:"field"` for binding, `validate:"required,..."` for rules.
func BindJSON(c *gin.Context, obj any) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		return mkerrors.Define(400, "BAD_REQUEST", err.Error())
	}
	return Struct(obj)
}

// BindForm binds form data (application/x-www-form-urlencoded or multipart/form-data)
// into obj and validates it.
// Use struct tags: `form:"field"` for binding, `validate:"required,..."` for rules.
func BindForm(c *gin.Context, obj any) error {
	if err := c.ShouldBind(obj); err != nil {
		return mkerrors.Define(400, "BAD_REQUEST", err.Error())
	}
	return Struct(obj)
}

// BindQuery binds URL query parameters into obj and validates it.
// Use struct tags: `form:"field"` for binding, `validate:"required,..."` for rules.
//
//	type ListReq struct {
//	    Page int `form:"page" validate:"min=1"`
//	}
func BindQuery(c *gin.Context, obj any) error {
	if err := c.ShouldBindQuery(obj); err != nil {
		return mkerrors.Define(400, "BAD_REQUEST", err.Error())
	}
	return Struct(obj)
}

// BindURI binds URI path parameters into obj and validates it.
// Use struct tags: `uri:"param"` for binding, `validate:"required,..."` for rules.
//
//	type UriReq struct {
//	    ID int64 `uri:"id" validate:"required,min=1"`
//	}
func BindURI(c *gin.Context, obj any) error {
	if err := c.ShouldBindUri(obj); err != nil {
		return mkerrors.Define(400, "BAD_REQUEST", err.Error())
	}
	return Struct(obj)
}

// toCodedError converts validator.ValidationErrors into a 400 CodedError
// with a metadata map of field → failed tag.
func toCodedError(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return mkerrors.Define(400, "VALIDATION_FAILED", err.Error())
	}
	metadata := make(map[string]any, len(ve))
	for _, fe := range ve {
		field := strings.ToLower(fe.Field())
		metadata[field] = fe.Tag()
	}
	ce := mkerrors.Define(400, "VALIDATION_FAILED", "validation failed")
	ce.Metadata = metadata
	return ce
}
