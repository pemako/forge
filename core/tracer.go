// core/tracer.go
package core

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Provider wraps an OpenTelemetry TracerProvider and adds a Shutdown method
// for clean resource release. *tracing.Provider implements this interface.
// Use tracing.New to obtain a concrete implementation.
type Provider interface {
	// Tracer returns a named trace.Tracer from the underlying otel provider.
	// name is typically the package import path of the instrumented component.
	Tracer(name string) trace.Tracer
	// Shutdown flushes pending spans and releases exporter resources.
	// Register with app.WithAfterStop to ensure spans are flushed on exit.
	Shutdown(ctx context.Context) error
}
