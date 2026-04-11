// core/server.go
package core

import "context"

// Server represents any long-running server (HTTP, gRPC, etc.).
// Start and Stop must be safe to call concurrently.
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
