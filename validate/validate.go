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
// Must be called before the first use of Bind or Struct.
func Register(tag string, fn validator.Func) error {
	return getInstance().RegisterValidation(tag, fn)
}

// Struct validates a struct using its `validate` tags.
// Returns a 400 CodedError with field-level details on failure.
func Struct(obj any) error {
	if err := getInstance().Struct(obj); err != nil {
		return toCodedError(err)
	}
	return nil
}

// Bind deserialises the JSON request body into obj and validates it.
// Returns a 400 CodedError on bind or validation failure.
// Use in gin handlers: if err := validate.Bind(c, &req); err != nil { c.Error(err); return }
func Bind(c *gin.Context, obj any) error {
	if err := c.ShouldBindJSON(obj); err != nil {
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
