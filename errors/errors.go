// Package errors provides structured HTTP-aware errors for mk services.
// Errors carry an HTTP status code and a machine-readable reason string so
// that middleware can render consistent JSON error responses without leaking
// internal details.
//
// Typical usage:
//
//	var ErrUserNotFound = errors.Define(404, "USER_NOT_FOUND", "user not found")
//
//	func GetUser(id string) error {
//	    return errors.Wrap(ErrUserNotFound, "id=%s", id)
//	}
package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// CodedError is a structured error with an HTTP status code and a machine-readable
// reason string. Use Define to create package-level sentinels and Wrap to attach
// context before returning from a handler.
type CodedError struct {
	// Code is the HTTP status code (e.g. 400, 404, 500).
	Code int `json:"code"`
	// Reason is the machine-readable error token (e.g. "NOT_FOUND").
	Reason string `json:"reason"`
	// Message is the human-readable description returned to the caller.
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
// Two CodedErrors are considered equal when their Reason fields match.
func (e *CodedError) Is(target error) bool {
	t, ok := target.(*CodedError)
	if !ok {
		return false
	}
	return e.Reason == t.Reason
}

// Define creates a new sentinel CodedError with the given HTTP status code,
// machine-readable reason, and default human-readable message.
// Store the result in a package-level var and pass it to Wrap when returning errors.
func Define(code int, reason, message string) *CodedError {
	return &CodedError{Code: code, Reason: reason, Message: message}
}

// Wrap returns a new CodedError that copies the sentinel's code and reason,
// with msg and args formatted as the cause. Use %w in msg to wrap an underlying
// error so that errors.Is and errors.As chains work correctly.
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

// Code returns the HTTP status code from err. If err is not a *CodedError
// (or does not wrap one), Code returns 500 (Internal Server Error).
func Code(err error) int {
	var ce *CodedError
	if stderrors.As(err, &ce) {
		return ce.Code
	}
	return http.StatusInternalServerError
}

// Reason returns the machine-readable reason string from err. If err is not a
// *CodedError (or does not wrap one), Reason returns "INTERNAL".
func Reason(err error) string {
	var ce *CodedError
	if stderrors.As(err, &ce) {
		return ce.Reason
	}
	return "INTERNAL"
}

// As is a convenience wrapper around stdlib errors.As.
// It reports whether any error in err's chain matches target, and if so sets
// target to that error value.
func As(err error, target any) bool { return stderrors.As(err, target) }

// Is is a convenience wrapper around stdlib errors.Is.
// It reports whether any error in err's chain matches target, using CodedError.Is
// for *CodedError comparisons (which matches on Reason, not pointer equality).
func Is(err, target error) bool { return stderrors.Is(err, target) }

// Predefined sentinel errors covering the most common HTTP failure cases.
// Use Wrap to attach request-specific context before returning these values.
var (
	// ErrBadRequest indicates HTTP 400: the request was malformed or failed validation.
	ErrBadRequest = Define(400, "BAD_REQUEST", "invalid request")
	// ErrUnAuth indicates HTTP 401: the caller is not authenticated.
	ErrUnAuth = Define(401, "UNAUTHORIZED", "please login")
	// ErrForbidden indicates HTTP 403: the caller is authenticated but lacks permission.
	ErrForbidden = Define(403, "FORBIDDEN", "permission denied")
	// ErrNotFound indicates HTTP 404: the requested resource does not exist.
	ErrNotFound = Define(404, "NOT_FOUND", "resource not found")
	// ErrConflict indicates HTTP 409: the request conflicts with the current state of the resource.
	ErrConflict = Define(409, "CONFLICT", "resource conflict")
	// ErrTooMany indicates HTTP 429: the caller has exceeded its rate limit.
	ErrTooMany = Define(429, "TOO_MANY_REQUEST", "too many requests")
	// ErrInternal indicates HTTP 500: an unexpected server-side error occurred.
	ErrInternal = Define(500, "INTERNAL", "internal server error")
)
