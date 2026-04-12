// Package core/server.go
package core

import "context"

// Server represents any long-running server (HTTP, gRPC, etc.).
// Implementations must be safe to call Start and Stop concurrently.
// Use server/http.New or server/grpc.New to obtain a concrete implementation.
type Server interface {
	// Start begins accepting connections and blocks until the server stops.
	// It returns a non-nil error if the server fails to start or encounters
	// a fatal error; returning http.ErrServerClosed is not considered an error.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the server within the deadline carried by ctx.
	Stop(ctx context.Context) error
}
