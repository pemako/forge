// errors/errors.go
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// CodedError is a structured error with an HTTP status code and a reason string.
// Use Define() to create sentinels; use Wrap() to attach context before returning.
type CodedError struct {
	Code     int            `json:"code"`
	Reason   string         `json:"reason"`
	Message  string         `json:"message"`
	Metadata map[string]any `json:"metadata,omitempty"`
	cause    error
}

func (e *CodedError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

func (e *CodedError) Unwrap() error { return e.cause }

// Is matches by Reason so stderrors.Is(wrapped, sentinel) works after Wrap.
func (e *CodedError) Is(target error) bool {
	t, ok := target.(*CodedError)
	if !ok {
		return false
	}
	return e.Reason == t.Reason
}

// Define creates a new sentinel CodedError. Store the result in a package-level var.
func Define(code int, reason, message string) *CodedError {
	return &CodedError{Code: code, Reason: reason, Message: message}
}

// Wrap returns a new CodedError with the sentinel's code/reason, attaching a message.
// msg supports fmt format verbs; use %w to wrap an underlying error so Is/As chain works.
func Wrap(sentinel *CodedError, msg string, args ...any) *CodedError {
	ce := &CodedError{
		Code:    sentinel.Code,
		Reason:  sentinel.Reason,
		Message: sentinel.Message,
	}
	if len(args) > 0 {
		ce.cause = fmt.Errorf(msg, args...)
	} else if msg != "" {
		ce.cause = stderrors.New(msg)
	}
	return ce
}

// Code returns the HTTP status code from err, or 500 if not a *CodedError.
func Code(err error) int {
	var ce *CodedError
	if stderrors.As(err, &ce) {
		return ce.Code
	}
	return http.StatusInternalServerError
}

// Reason returns the reason string from err, or "INTERNAL" if not a *CodedError.
func Reason(err error) string {
	var ce *CodedError
	if stderrors.As(err, &ce) {
		return ce.Reason
	}
	return "INTERNAL"
}

// As is a convenience wrapper around stdlib errors.As.
func As(err error, target any) bool { return stderrors.As(err, target) }

// Is is a convenience wrapper around stdlib errors.Is.
func Is(err, target error) bool { return stderrors.Is(err, target) }

// Predefined sentinel errors. Use Wrap() to attach context before returning.
var (
	ErrBadRequest = Define(400, "BAD_REQUEST",      "invalid request")
	ErrUnAuth     = Define(401, "UNAUTHORIZED",     "please login")
	ErrForbidden  = Define(403, "FORBIDDEN",        "permission denied")
	ErrNotFound   = Define(404, "NOT_FOUND",        "resource not found")
	ErrConflict   = Define(409, "CONFLICT",         "resource conflict")
	ErrTooMany    = Define(429, "TOO_MANY_REQUEST",  "too many requests")
	ErrInternal   = Define(500, "INTERNAL",         "internal server error")
)
