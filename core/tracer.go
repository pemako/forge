// core/tracer.go
package core

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Provider wraps an otel TracerProvider and adds Shutdown.
// *tracing.Provider implements this interface.
type Provider interface {
	// Tracer returns a named trace.Tracer from the underlying otel provider.
	Tracer(name string) trace.Tracer
	// Shutdown flushes pending spans. Register with app.WithAfterStop.
	Shutdown(ctx context.Context) error
}
