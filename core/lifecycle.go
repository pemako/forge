// Package core holds the interfaces shared across forge packages. It imports no
// other forge package, so anything here can be depended on freely.
package core

import "context"

// Migrator applies schema migrations before any server starts.
// *database.Manager implements it.
//
// It lives here, rather than in app, so a component can assert that it
// satisfies the contract at compile time — app may not be imported by library
// packages, which previously left these contracts satisfiable only by accident.
type Migrator interface {
	// Up applies all pending migrations. It is called once during startup,
	// before BeforeStart hooks and server initialization.
	Up(ctx context.Context) error
}

// Lifecycle manages a resource that needs setting up before servers start and
// tearing down after they stop.
//
// Init runs after migrations and before BeforeStart hooks. If it fails, every
// component already initialised is Shutdown in reverse order and every Closer is
// Closed before startup is abandoned.
//
// Shutdown runs after all servers have stopped, in reverse registration order.
// tracing.Provider and metric.Provider implement it.
type Lifecycle interface {
	Init(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// Closer releases resources after all servers have stopped and every
// Lifecycle.Shutdown has completed. Close runs in reverse registration order,
// matching Lifecycle, so a component may depend on one registered before it.
//
// Close takes no context and so cannot be cancelled. The caller bounds the wait
// and moves on if Close does not return in time, which leaks that goroutine —
// implementations should return promptly.
type Closer interface {
	Close() error
}

// ReadinessToggler is an optional interface a Server may implement to expose its
// readiness flag. Servers implementing it are marked ready once all servers have
// started and not-ready as soon as shutdown begins, so a load balancer stops
// routing new requests into a draining instance.
type ReadinessToggler interface {
	SetReady(ready bool)
}

// ReadyCheckRegistrar is an optional interface a Server may implement to accept
// readiness probes, so registered HealthCheckers can be wired in automatically
// before the server starts serving.
type ReadyCheckRegistrar interface {
	AddReadyCheck(name string, fn func(context.Context) error)
}
