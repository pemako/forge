// Package core defines the fundamental interfaces used throughout mk.
// All concrete implementations (logger, config, server, tracing) satisfy
// these interfaces, enabling dependency injection and easy substitution in tests.
package core

import "context"

// Field is a key-value pair for structured log entries.
// Use logger.F to construct fields concisely.
type Field struct {
	Key   string
	Value any
}

// Logger is the interface all logger implementations must satisfy.
// Every method accepts a context so that trace_id and span_id can be
// automatically injected from an active OpenTelemetry span.
// Use logger.New to obtain a concrete implementation.
type Logger interface {
	// Debug logs a message at DEBUG level with optional structured fields.
	Debug(ctx context.Context, msg string, fields ...Field)
	// Info logs a message at INFO level with optional structured fields.
	Info(ctx context.Context, msg string, fields ...Field)
	// Warn logs a message at WARN level with optional structured fields.
	Warn(ctx context.Context, msg string, fields ...Field)
	// Error logs a message at ERROR level with optional structured fields.
	Error(ctx context.Context, msg string, fields ...Field)
	// With returns a new Logger with the given fields pre-attached to
	// every subsequent log entry.
	With(fields ...Field) Logger
}
