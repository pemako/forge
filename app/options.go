// Package app provides the application lifecycle manager for mk services.
// It coordinates startup (migrations, hooks, servers) and graceful shutdown
// in response to SIGTERM or SIGINT, giving each registered server time to
// drain in-flight requests.
//
// # Design boundary: orchestrator, not container
//
// app is a process lifecycle orchestrator. It sequences the startup and
// shutdown of components that are passed into it. It does not own, discover,
// or vend those components.
//
// app will never:
//   - Act as a global dependency registry or service locator
//   - Distribute configuration to registered components
//   - Route requests or messages between business modules
//   - Support dynamic component registration after Run() starts
//   - Replace constructor injection with runtime lookup
//
// If you find yourself wanting app to hold a reference so other code can
// retrieve it later, that is a sign the dependency should be injected
// directly at construction time instead.
//
// Typical usage:
//
//	a := app.New(
//	    app.WithName("my-service"),
//	    app.WithServer(httpSrv),
//	    app.WithMigration(dbManager),
//	)
//	if err := a.Run(); err != nil {
//	    log.Fatal(err)
//	}
package app

import (
	"context"
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
)

// The lifecycle contracts live in core so that library packages — which must
// not import app — can assert they satisfy them at compile time. These aliases
// keep the app.Lifecycle spelling working.
type (
	// Migrator applies schema migrations before any server starts.
	// See core.Migrator.
	Migrator = core.Migrator

	// Lifecycle manages a resource set up before servers start and torn down
	// after they stop. See core.Lifecycle.
	Lifecycle = core.Lifecycle

	// Closer releases resources after all Lifecycle shutdowns complete.
	// See core.Closer.
	Closer = core.Closer

	// ReadinessToggler is an optional Server interface letting app drive the
	// readiness flag. See core.ReadinessToggler.
	ReadinessToggler = core.ReadinessToggler

	// ReadyCheckRegistrar is an optional Server interface accepting readiness
	// probes. See core.ReadyCheckRegistrar.
	ReadyCheckRegistrar = core.ReadyCheckRegistrar
)

// options holds the functional configuration for New.
type options struct {
	name            string
	version         string
	servers         []core.Server
	logger          core.Logger
	migrator        Migrator
	lifecycles      []Lifecycle
	closers         []Closer
	checkers        []core.HealthChecker
	shutdownTimeout time.Duration
	drainDelay      time.Duration
	beforeStart     []func(context.Context) error
	afterStop       []func(context.Context) error
}

func defaultOptions() options {
	return options{
		name:            "app",
		version:         "unknown",
		shutdownTimeout: 30 * time.Second,
		logger:          logger.Default(),
	}
}

// Option is a functional option for configuring the App.
type Option func(*options)

// WithName sets the application name. It is included in the "app: starting" and
// "app: shutdown complete" log lines. Defaults to "app".
func WithName(name string) Option { return func(o *options) { o.name = name } }

// WithVersion sets the application version string. It is included in the
// "app: starting" and "app: shutdown complete" log lines. Defaults to "unknown".
func WithVersion(v string) Option { return func(o *options) { o.version = v } }

// WithLogger sets the logger used for lifecycle events. Defaults to
// logger.Default(), which discards everything until logger.SetDefault is called
// — so pass this (or call SetDefault) if you want to see lifecycle logs.
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithServer registers one or more servers to be started concurrently by Run.
// All registered servers are stopped on shutdown.
func WithServer(servers ...core.Server) Option {
	return func(o *options) { o.servers = append(o.servers, servers...) }
}

// WithShutdownTimeout sets the budget for stopping all servers gracefully.
// Each post-server phase (Lifecycle.Shutdown, Closer.Close, AfterStop) gets
// whatever is left of this budget, floored at 5 seconds so a slow drain cannot
// starve telemetry flushing. Defaults to 30 seconds.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) { o.shutdownTimeout = d }
}

// WithDrainDelay sets how long Run waits, after marking servers not-ready but
// before stopping them, so load balancers observe the failing readiness probe
// and stop sending new requests. Defaults to 0 (no wait).
//
// Set this to a small multiple of your readiness probe interval — in Kubernetes,
// roughly periodSeconds × failureThreshold — and keep
// WithDrainDelay + WithShutdownTimeout below terminationGracePeriodSeconds.
func WithDrainDelay(d time.Duration) Option {
	return func(o *options) { o.drainDelay = d }
}

// WithMigration registers a Migrator whose Up method is called once before
// BeforeStart hooks and server startup. Use this to run database migrations.
func WithMigration(m Migrator) Option {
	return func(o *options) { o.migrator = m }
}

// WithBeforeStart registers a hook that is called after migrations but before
// any servers are started. Multiple hooks are executed in registration order.
// If a hook returns an error, Run aborts immediately.
func WithBeforeStart(fn func(context.Context) error) Option {
	return func(o *options) { o.beforeStart = append(o.beforeStart, fn) }
}

// WithAfterStop registers a hook that is called after all servers have stopped.
// Use this to flush telemetry or release resources (e.g., tracing.Provider.Shutdown).
// Multiple hooks are executed in registration order.
func WithAfterStop(fn func(context.Context) error) Option {
	return func(o *options) { o.afterStop = append(o.afterStop, fn) }
}

// WithLifecycle registers one or more Lifecycle components. Init is called
// after migrations and before BeforeStart hooks, in registration order.
// Shutdown is called after all servers have stopped, in reverse registration order.
func WithLifecycle(components ...Lifecycle) Option {
	return func(o *options) { o.lifecycles = append(o.lifecycles, components...) }
}

// WithCloser registers one or more Closer instances to be called after all
// Lifecycle Shutdown calls have completed, in reverse registration order.
// Errors are collected but do not stop subsequent Closers.
func WithCloser(closers ...Closer) Option {
	return func(o *options) { o.closers = append(o.closers, closers...) }
}

// WithHealthCheck registers one or more HealthChecker instances with the App.
// Run registers each of them as a readiness probe on every server that
// implements ReadyCheckRegistrar, before any server starts — so for the
// built-in HTTP and gRPC servers no further wiring is needed:
//
//	app.New(
//	    app.WithServer(httpSrv),
//	    app.WithHealthCheck(dbMgr.HealthChecker("default")),
//	)
//
// If no registered server accepts probes, Run logs that the checkers went
// nowhere rather than failing silently.
func WithHealthCheck(checkers ...core.HealthChecker) Option {
	return func(o *options) { o.checkers = append(o.checkers, checkers...) }
}
