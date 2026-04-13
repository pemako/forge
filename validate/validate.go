// Package validate provides request binding and struct validation helpers
// for gin handlers, backed by go-playground/validator/v10. Each Bind* function
// combines gin's ShouldBind* for deserialization with Struct for tag-driven
// validation, returning a single 400 CodedError with field-level detail on
// failure. Struct can also be used standalone in the service layer.
//
// All functions share a singleton *validator.Validate instance that is
// initialised on first use. Call Register before the first Bind* or Struct
// call to add custom validation tags.
//
// Typical usage:
//
//	type CreateUserReq struct {
//	    Name  string `json:"name"  validate:"required,min=2,max=64"`
//	    Email string `json:"email" validate:"required,email"`
//	}
//
//	func CreateUser(c *gin.Context) {
//	    var req CreateUserReq
//	    if err := validate.BindJSON(c, &req); err != nil {
//	        c.Error(err)
//	        return
//	    }
//	    // req is safe to use
//	}
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
	// ErrValidationFailed is the sentinel for validation errors.
	// Callers can use errors.Is(err, validate.ErrValidationFailed) to check.
	ErrValidationFailed = mkerrors.Define(400, "VALIDATION_FAILED", "validation failed")
	// ErrBadRequest is used when binding fails (malformed JSON, etc.).
	ErrBadRequest = mkerrors.Define(400, "BAD_REQUEST", "bad request")
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

// Register adds a custom validation function for the given tag to the shared
// validator instance. It must be called before the first invocation of any
// Bind* or Struct function; later calls have no effect on the already-
// initialised singleton.
//
//	validate.Register("slug", func(fl validator.FieldLevel) bool {
//	    return slugRe.MatchString(fl.Field().String())
//	})
func Register(tag string, fn validator.Func) error {
	return getInstance().RegisterValidation(tag, fn)
}

// Struct validates a struct using its `validate` struct tags. It returns a 400
// CodedError whose Metadata map contains field → failed-tag pairs on failure,
// or nil on success. Unlike the Bind* functions, Struct does not perform any
// binding; it can be called outside gin handlers (e.g., in the service layer)
// to re-validate or validate values constructed in code.
func Struct(obj any) error {
	if err := getInstance().Struct(obj); err != nil {
		return toCodedError(err)
	}
	return nil
}

// Bind binds the request body based on Content-Type and validates the result.
// Supports JSON (application/json) and form data (application/x-www-form-urlencoded,
// multipart/form-data). Use BindJSON or BindForm to bind to a specific format
// regardless of Content-Type.
func Bind(c *gin.Context, obj any) error {
	if err := c.ShouldBind(obj); err != nil {
		return mkerrors.Wrap(ErrBadRequest, "%s", err.Error())
	}
	return Struct(obj)
}

// BindJSON binds the JSON request body into obj and validates it.
// Use struct tags: `json:"field"` for binding, `validate:"required,..."` for rules.
func BindJSON(c *gin.Context, obj any) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		return mkerrors.Wrap(ErrBadRequest, "%s", err.Error())
	}
	return Struct(obj)
}

// BindForm binds form data (application/x-www-form-urlencoded or multipart/form-data)
// into obj and validates it.
// Use struct tags: `form:"field"` for binding, `validate:"required,..."` for rules.
func BindForm(c *gin.Context, obj any) error {
	if err := c.ShouldBind(obj); err != nil {
		return mkerrors.Wrap(ErrBadRequest, "%s", err.Error())
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
		return mkerrors.Wrap(ErrBadRequest, "%s", err.Error())
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
		return mkerrors.Wrap(ErrBadRequest, "%s", err.Error())
	}
	return Struct(obj)
}

// toCodedError converts validator.ValidationErrors into a 400 CodedError
// with a metadata map of field → failed tag.
func toCodedError(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return mkerrors.Wrap(ErrValidationFailed, "%s", err.Error())
	}
	metadata := make(map[string]any, len(ve))
	for _, fe := range ve {
		metadata[strings.ToLower(fe.Field())] = fe.Tag()
	}
	wrapped := mkerrors.Wrap(ErrValidationFailed, "validation failed")
	wrapped.Metadata = metadata
	return wrapped
}
