// errors/errors_test.go
package errors_test

import (
	stderrors "errors"
	"testing"

	"github.com/pemako/mk/errors"
	"github.com/stretchr/testify/assert"
)

func TestDefine(t *testing.T) {
	e := errors.Define(404, "NOT_FOUND", "resource not found")
	assert.Equal(t, 404, e.Code)
	assert.Equal(t, "NOT_FOUND", e.Reason)
	assert.Equal(t, "resource not found", e.Error())
}

func TestWrap_messageOnly(t *testing.T) {
	sentinel := errors.Define(404, "USER_NOT_FOUND", "user not found")
	wrapped := errors.Wrap(sentinel, "query user")
	assert.Equal(t, 404, errors.Code(wrapped))
	assert.Equal(t, "USER_NOT_FOUND", errors.Reason(wrapped))
	assert.True(t, stderrors.Is(wrapped, sentinel))
}

func TestWrap_withCause(t *testing.T) {
	sentinel := errors.Define(404, "USER_NOT_FOUND", "user not found")
	cause := stderrors.New("db timeout")
	wrapped := errors.Wrap(sentinel, "query user: %w", cause)
	assert.True(t, stderrors.Is(wrapped, sentinel))
	assert.True(t, stderrors.Is(wrapped, cause))
}

func TestCode_nonCodedError(t *testing.T) {
	assert.Equal(t, 500, errors.Code(stderrors.New("oops")))
}

func TestReason_nonCodedError(t *testing.T) {
	assert.Equal(t, "INTERNAL", errors.Reason(stderrors.New("oops")))
}

func TestAs(t *testing.T) {
	e := errors.Define(403, "FORBIDDEN", "forbidden")
	var ce *errors.CodedError
	assert.True(t, errors.As(e, &ce))
	assert.Equal(t, 403, ce.Code)
}

func TestIs(t *testing.T) {
	assert.True(t, errors.Is(errors.ErrNotFound, errors.ErrNotFound))
	assert.False(t, errors.Is(errors.ErrNotFound, errors.ErrInternal))
}

func TestSentinelVars(t *testing.T) {
	assert.Equal(t, 400, errors.ErrBadRequest.Code)
	assert.Equal(t, 401, errors.ErrUnAuth.Code)
	assert.Equal(t, 403, errors.ErrForbidden.Code)
	assert.Equal(t, 404, errors.ErrNotFound.Code)
	assert.Equal(t, 409, errors.ErrConflict.Code)
	assert.Equal(t, 429, errors.ErrTooMany.Code)
	assert.Equal(t, 500, errors.ErrInternal.Code)
}
