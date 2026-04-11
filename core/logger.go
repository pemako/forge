// core/logger.go
package core

import "context"

// Field is a key-value pair for structured log entries.
type Field struct {
	Key   string
	Value any
}

// Logger is the interface all logger implementations must satisfy.
// ctx is required on every call so trace_id can be injected automatically.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)
	With(fields ...Field) Logger
}
