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

	"github.com/pemako/mk/core"
)

// Migrator is the interface app accepts for running database migrations before
// servers are started. *database.Manager implements this interface.
// It is defined here (rather than in the database package) to avoid import cycles.
type Migrator interface {
	// Up applies all pending migrations. It is called once during app startup,
	// before BeforeStart hooks and server initialization.
	Up(ctx context.Context) error
}

// Lifecycle manages the lifecycle of a non-blocking resource.
// Init is called after migrations and before BeforeStart hooks.
// If Init fails, all previously Init'd Lifecycle components are Shutdown in
// reverse order before Run returns the error.
// Shutdown is called after all servers have stopped, in reverse registration order.
type Lifecycle interface {
	Init(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// Closer releases resources after all servers have stopped and all Lifecycle
// Shutdown calls have completed. Close is called in registration order.
// Errors are collected but do not stop subsequent Closers.
type Closer interface {
	Close() error
}

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
	beforeStart     []func(context.Context) error
	afterStop       []func(context.Context) error
}

// Option is a functional option for configuring the App.
type Option func(*options)

// WithName sets the application name used in log output. Defaults to "app".
func WithName(name string) Option { return func(o *options) { o.name = name } }

// WithVersion sets the application version string used in log output. Defaults to "unknown".
func WithVersion(v string) Option { return func(o *options) { o.version = v } }

// WithLogger sets the logger used for lifecycle events. Defaults to logger.Default().
func WithLogger(l core.Logger) Option { return func(o *options) { o.logger = l } }

// WithServer registers one or more servers to be started concurrently by Run.
// All registered servers are stopped on shutdown.
func WithServer(servers ...core.Server) Option {
	return func(o *options) { o.servers = append(o.servers, servers...) }
}

// WithShutdownTimeout sets the maximum time allowed for all servers to stop
// gracefully. Defaults to 30 seconds.
func WithShutdownTimeout(d time.Duration) Option {
	return func(o *options) { o.shutdownTimeout = d }
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
// Lifecycle Shutdown calls have completed, in registration order.
// Errors are collected but do not stop subsequent Closers.
func WithCloser(closers ...Closer) Option {
	return func(o *options) { o.closers = append(o.closers, closers...) }
}

// WithHealthCheck registers one or more HealthChecker instances with the App.
// The app does not automatically wire these into any server; callers should
// bridge them to server/http in a WithBeforeStart hook:
//
//	app.New(
//	    app.WithHealthCheck(dbMgr.HealthChecker("default")),
//	    app.WithBeforeStart(func(ctx context.Context) error {
//	        for _, hc := range a.HealthCheckers() {
//	            httpSrv.AddReadyCheck(hc.Name(), hc.Check)
//	        }
//	        return nil
//	    }),
//	)
func WithHealthCheck(checkers ...core.HealthChecker) Option {
	return func(o *options) { o.checkers = append(o.checkers, checkers...) }
}
