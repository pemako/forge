package app

import (
	stderrors "errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"sync"
	"syscall"
	"time"

	"context"

	"github.com/pemako/forge/core"
	ferrors "github.com/pemako/forge/errors"
)

// minPhaseTimeout is the budget guaranteed to each shutdown phase that runs
// after the server drain. The drain is allowed to consume the whole
// shutdownTimeout, so without a floor here a slow drain would hand an
// already-expired context to Lifecycle.Shutdown and Closer.Close — which in
// practice means telemetry never gets flushed. Each later phase therefore gets
// whatever is left of the budget, or this floor, whichever is larger.
const minPhaseTimeout = 5 * time.Second

// ErrPanic is returned when a lifecycle callback, hook, or server goroutine
// panics. The panic is recovered and converted to this error so the remaining
// shutdown phases still run instead of the process dying with resources held.
var ErrPanic = ferrors.Define(500, "APP_PANIC", "panic recovered in lifecycle callback")

// ErrServersExited is returned when every registered server's Start returned
// without an error and without a shutdown having been requested. Start must
// block for the lifetime of the server (see core.Server); a Start that returns
// early leaves the process running with nothing serving traffic.
var ErrServersExited = ferrors.Define(500, "APP_SERVERS_EXITED", "all servers exited unexpectedly")

// App is the application lifecycle manager. Create one with New, then call Run.
type App struct {
	opts     options
	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates an App with the given options. The following defaults are applied
// when options are not provided: name="app", version="unknown",
// shutdownTimeout=30s, logger=logger.Default().
func New(opts ...Option) *App {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &App{opts: o, stopCh: make(chan struct{})}
}

// Stop requests a graceful shutdown of a running App. It returns immediately;
// Run performs the shutdown and returns once it completes.
//
// Stop is safe to call from any goroutine, more than once, and before Run has
// started — in the last case Run shuts down as soon as startup finishes. Use it
// to end a service from inside the process, and in tests to stop an App without
// signalling the whole process.
func (a *App) Stop() {
	a.stopOnce.Do(func() { close(a.stopCh) })
}

// log emits a structured Info log through the configured logger.
func (a *App) log(ctx context.Context, msg string, fields ...core.Field) {
	if a.opts.logger != nil {
		a.opts.logger.Info(ctx, msg, fields...)
	}
}

// logError emits a structured Error log through the configured logger.
func (a *App) logError(ctx context.Context, msg string, fields ...core.Field) {
	if a.opts.logger != nil {
		a.opts.logger.Error(ctx, msg, fields...)
	}
}

// HealthCheckers returns the registered health checkers.
//
// Run already registers these on every server that accepts readiness probes, so
// callers rarely need this. It remains available for wiring checkers somewhere
// app does not reach.
func (a *App) HealthCheckers() []core.HealthChecker {
	return a.opts.checkers
}

// guard runs fn and converts a panic into an error. Without this a panic in any
// user-supplied callback or server goroutine would kill the process with
// already-initialised components still holding their resources.
func (a *App) guard(ctx context.Context, phase string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			a.logError(ctx, "app: panic recovered",
				core.Field{Key: "phase", Value: phase},
				core.Field{Key: "panic", Value: fmt.Sprint(r)},
				core.Field{Key: "stack", Value: string(debug.Stack())},
			)
			err = fmt.Errorf("%s: %w: %v", phase, ErrPanic, r)
		}
	}()
	return fn()
}

// Run starts the application lifecycle and blocks until the process receives
// SIGTERM or SIGINT, Stop is called, or a server fails.
//
// It is equivalent to RunContext(context.Background()).
func (a *App) Run() error { return a.RunContext(context.Background()) }

// RunContext starts the application lifecycle and blocks until ctx is
// cancelled, the process receives SIGTERM or SIGINT, Stop is called, or a
// server fails. Cancelling ctx requests the same graceful shutdown a signal
// would; the shutdown sequence itself is not bound by ctx, so cancelling it
// does not cut the drain short.
//
// Phases, in order:
//
//  1. Readiness probes from WithHealthCheck are registered on every server that
//     accepts them. This happens before any server starts, so the probe list is
//     never mutated while requests are being served.
//  2. Migrations (WithMigration) — Up is called once.
//  3. Lifecycle.Init — sequential.
//  4. BeforeStart hooks — sequential.
//  5. All registered servers start concurrently and are marked ready.
//  6. Run blocks until shutdown is requested or a server fails.
//  7. Servers are marked not-ready and, if WithDrainDelay is set, Run waits
//     that long before stopping them so load balancers notice first.
//  8. Servers are stopped concurrently within the shutdown budget.
//  9. Lifecycle.Shutdown — reverse registration order.
//  10. Closer.Close — reverse registration order.
//  11. AfterStop hooks — sequential.
//
// If startup fails at phase 2, 3 or 4, every component already brought up is
// torn down — Lifecycles are Shutdown in reverse and Closers are Closed — before
// Run returns, so an aborted startup does not leak connections.
//
// A panic in any phase is recovered, logged with its stack, and reported as
// ErrPanic; the remaining shutdown phases still run.
//
// A second SIGTERM or SIGINT during shutdown abandons the remaining waits and
// returns as soon as possible, so an operator is never left with a process that
// ignores signals.
//
// Run returns a non-nil error if any phase fails. Errors from independent
// components are joined, and each is wrapped with the phase that produced it.
func (a *App) RunContext(ctx context.Context) error {
	// Shutdown must survive the cancellation that triggered it, so its phases
	// hang off a context that keeps ctx's values but not its cancellation.
	shutdownParent := context.WithoutCancel(ctx)

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// Signals are trapped before any startup work. Previously signal.Notify ran
	// only after the servers were launched, so a SIGTERM arriving during
	// migrations hit the default disposition and killed the process mid-migration.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	forceCh := make(chan struct{})
	watcherDone := make(chan struct{})
	defer close(watcherDone)

	go func() {
		// Each of these cases stays ready once it has fired, so the channel is
		// set to nil after firing to take the case out of the select. Without
		// that, the loop spins hot on the already-cancelled context.
		stopCh := a.stopCh
		ctxDone := runCtx.Done()
		requested := false
		request := func(reason string) {
			if requested {
				return
			}
			requested = true
			a.log(shutdownParent, "app: shutdown requested", core.Field{Key: "reason", Value: reason})
			cancelRun()
		}
		for {
			select {
			case <-sigCh:
				if requested {
					a.log(shutdownParent, "app: second signal received, abandoning graceful shutdown")
					close(forceCh)
					return
				}
				request("signal")
			case <-stopCh:
				stopCh = nil
				request("stop")
			case <-ctxDone:
				ctxDone = nil
				request("context cancelled")
			case <-watcherDone:
				return
			}
		}
	}()

	a.log(runCtx, "app: starting",
		core.Field{Key: "name", Value: a.opts.name},
		core.Field{Key: "version", Value: a.opts.version},
	)

	// Phase 1 — register readiness probes before anything serves traffic.
	a.registerReadyChecks(runCtx)

	// Phases 2-4 — startup. On failure everything already up is torn down.
	initialised := 0
	startupErr := func() error {
		if a.opts.migrator != nil {
			if err := a.guard(runCtx, "migration", func() error {
				return a.opts.migrator.Up(runCtx)
			}); err != nil {
				return fmt.Errorf("migration: %w", err)
			}
			a.log(runCtx, "app: migrations complete")
		}
		if err := runCtx.Err(); err != nil {
			return fmt.Errorf("startup aborted: %w", err)
		}

		for i, lc := range a.opts.lifecycles {
			if err := a.guard(runCtx, "lifecycle init", func() error {
				return lc.Init(runCtx)
			}); err != nil {
				return fmt.Errorf("lifecycle init[%d]: %w", i, err)
			}
			initialised = i + 1
		}
		if err := runCtx.Err(); err != nil {
			return fmt.Errorf("startup aborted: %w", err)
		}

		for i, fn := range a.opts.beforeStart {
			if err := a.guard(runCtx, "before start", func() error {
				return fn(runCtx)
			}); err != nil {
				return fmt.Errorf("before start[%d]: %w", i, err)
			}
		}
		return runCtx.Err()
	}()

	if startupErr != nil {
		// Previously a BeforeStart failure returned here with Lifecycles still
		// initialised and Closers untouched, leaking every pool and client the
		// service had already opened.
		a.logError(shutdownParent, "app: startup failed, rolling back",
			core.Field{Key: "err", Value: startupErr.Error()})
		rollbackErr := a.teardown(shutdownParent, forceCh, initialised)
		if rollbackErr != nil {
			return stderrors.Join(startupErr, rollbackErr)
		}
		return startupErr
	}

	// Phase 5 — start servers.
	a.log(runCtx, "app: starting servers", core.Field{Key: "count", Value: len(a.opts.servers)})

	errCh := make(chan error, len(a.opts.servers)+1)
	var startWg sync.WaitGroup
	for _, srv := range a.opts.servers {
		startWg.Go(func() {
			if err := a.guard(runCtx, "server start", func() error {
				return srv.Start(runCtx)
			}); err != nil {
				errCh <- err
			}
		})
	}

	startsDone := make(chan struct{})
	go func() { startWg.Wait(); close(startsDone) }()

	// Only treat "every Start returned" as a failure when there are servers to
	// begin with. With none registered this channel stays nil so the select
	// below blocks until shutdown is requested, as an app with no servers should.
	var serversExited <-chan struct{}
	if len(a.opts.servers) > 0 {
		serversExited = startsDone
	}

	a.setReady(runCtx, true)

	// Phase 6 — wait for shutdown.
	var runErr error
	select {
	case err := <-errCh:
		runErr = fmt.Errorf("server error: %w", err)
		a.logError(shutdownParent, "app: server failed", core.Field{Key: "err", Value: err.Error()})
	case <-serversExited:
		// Every Start returned nil while the app was still meant to be serving.
		// core.Server requires Start to block, so this is a broken server, not a
		// shutdown — previously it left Run blocked forever with nothing serving.
		if runCtx.Err() == nil {
			runErr = fmt.Errorf("%w", ErrServersExited)
			a.logError(shutdownParent, "app: all servers exited without a shutdown request")
		}
	case <-runCtx.Done():
	}

	// Phase 7 — stop advertising readiness, then let load balancers react.
	a.setReady(shutdownParent, false)
	if a.opts.drainDelay > 0 {
		a.log(shutdownParent, "app: draining",
			core.Field{Key: "delay", Value: a.opts.drainDelay.String()})
		select {
		case <-time.After(a.opts.drainDelay):
		case <-forceCh:
		}
	}

	deadline := time.Now().Add(a.opts.shutdownTimeout)

	// Phase 8 — stop servers.
	stopCtx, cancelStop := context.WithDeadline(shutdownParent, deadline)
	defer cancelStop()

	stopErrs := make([]error, len(a.opts.servers))
	var stopWg sync.WaitGroup
	for i, srv := range a.opts.servers {
		stopWg.Go(func() {
			if err := a.guard(stopCtx, "server stop", func() error {
				return srv.Stop(stopCtx)
			}); err != nil {
				stopErrs[i] = err
				a.logError(shutdownParent, "app: server stop error",
					core.Field{Key: "err", Value: err.Error()})
			}
		})
	}
	stopDone := make(chan struct{})
	go func() { stopWg.Wait(); close(stopDone) }()
	select {
	case <-stopDone:
	case <-stopCtx.Done():
		a.log(shutdownParent, "app: shutdown timeout exceeded")
	case <-forceCh:
	}

	// Wait for Start goroutines to exit, bounded by the same deadline.
	select {
	case <-startsDone:
	case <-stopCtx.Done():
		a.log(shutdownParent, "app: start goroutines did not exit within shutdown timeout")
	case <-forceCh:
	}

	var stopJoined []error
	for _, err := range stopErrs {
		if err != nil {
			stopJoined = append(stopJoined, fmt.Errorf("server stop: %w", err))
		}
	}
	if len(stopJoined) > 0 {
		runErr = stderrors.Join(runErr, stderrors.Join(stopJoined...))
	}

	// Phases 9-11.
	if err := a.teardownFrom(shutdownParent, forceCh, len(a.opts.lifecycles), deadline); err != nil {
		runErr = stderrors.Join(runErr, err)
	}

	a.log(shutdownParent, "app: shutdown complete",
		core.Field{Key: "name", Value: a.opts.name},
		core.Field{Key: "version", Value: a.opts.version},
	)
	return runErr
}

// registerReadyChecks wires every checker from WithHealthCheck into each server
// that accepts readiness probes. WithHealthCheck previously stored the checkers
// and wired nothing, leaving callers to bridge them by hand.
func (a *App) registerReadyChecks(ctx context.Context) {
	if len(a.opts.checkers) == 0 {
		return
	}
	wired := 0
	for _, srv := range a.opts.servers {
		reg, ok := srv.(ReadyCheckRegistrar)
		if !ok {
			continue
		}
		for _, hc := range a.opts.checkers {
			reg.AddReadyCheck(hc.Name(), hc.Check)
		}
		wired++
	}
	if wired == 0 {
		a.log(ctx, "app: health checkers registered but no server accepts readiness probes",
			core.Field{Key: "checkers", Value: len(a.opts.checkers)})
		return
	}
	a.log(ctx, "app: readiness probes registered",
		core.Field{Key: "checkers", Value: len(a.opts.checkers)},
		core.Field{Key: "servers", Value: wired},
	)
}

// setReady flips the readiness flag on every server that exposes one. Marking
// not-ready on shutdown is what stops a load balancer routing new requests into
// a draining instance.
func (a *App) setReady(ctx context.Context, ready bool) {
	n := 0
	for _, srv := range a.opts.servers {
		if t, ok := srv.(ReadinessToggler); ok {
			t.SetReady(ready)
			n++
		}
	}
	if n > 0 {
		a.log(ctx, "app: readiness changed",
			core.Field{Key: "ready", Value: ready},
			core.Field{Key: "servers", Value: n},
		)
	}
}

// teardown runs the post-server phases with a fresh budget. Used on the startup
// failure path, where no drain has happened yet.
func (a *App) teardown(ctx context.Context, forceCh <-chan struct{}, initialised int) error {
	return a.teardownFrom(ctx, forceCh, initialised, time.Now().Add(a.opts.shutdownTimeout))
}

// teardownFrom shuts down the first `initialised` Lifecycles in reverse order,
// then the Closers in reverse order, then runs AfterStop hooks. Each phase gets
// its own context so an earlier overrun cannot starve a later phase.
func (a *App) teardownFrom(ctx context.Context, forceCh <-chan struct{}, initialised int, deadline time.Time) error {
	var errs []error

	lcCtx, cancelLC := a.phaseCtx(ctx, deadline)
	defer cancelLC()
	for i, lc := range slices.Backward(a.opts.lifecycles[:initialised]) {
		if err := a.guard(lcCtx, "lifecycle shutdown", func() error {
			return lc.Shutdown(lcCtx)
		}); err != nil {
			a.logError(ctx, "app: lifecycle shutdown error",
				core.Field{Key: "index", Value: i},
				core.Field{Key: "err", Value: err.Error()},
			)
			errs = append(errs, fmt.Errorf("lifecycle shutdown[%d]: %w", i, err))
		}
	}

	// Closers now run in reverse registration order, matching Lifecycle. Forward
	// order tore down a dependency before the Closer that used it.
	closeCtx, cancelClose := a.phaseCtx(ctx, deadline)
	defer cancelClose()
	for i, c := range slices.Backward(a.opts.closers) {
		// Closer.Close takes no context and cannot be cancelled, so it runs in
		// its own goroutine and the wait is bounded. A Close that never returns
		// leaks this goroutine, but no longer hangs the process forever.
		done := make(chan error, 1)
		go func() { done <- a.guard(closeCtx, "closer", c.Close) }()
		select {
		case err := <-done:
			if err != nil {
				a.logError(ctx, "app: closer error",
					core.Field{Key: "index", Value: i},
					core.Field{Key: "err", Value: err.Error()},
				)
				errs = append(errs, fmt.Errorf("closer[%d]: %w", i, err))
			}
		case <-closeCtx.Done():
			a.logError(ctx, "app: closer did not return within shutdown budget",
				core.Field{Key: "index", Value: i})
			errs = append(errs, fmt.Errorf("closer[%d]: %w", i, closeCtx.Err()))
		case <-forceCh:
			a.logError(ctx, "app: closer abandoned on forced shutdown",
				core.Field{Key: "index", Value: i})
		}
	}

	afterCtx, cancelAfter := a.phaseCtx(ctx, deadline)
	defer cancelAfter()
	for i, fn := range a.opts.afterStop {
		if err := a.guard(afterCtx, "after stop", func() error {
			return fn(afterCtx)
		}); err != nil {
			a.logError(ctx, "app: after stop error",
				core.Field{Key: "index", Value: i},
				core.Field{Key: "err", Value: err.Error()},
			)
			errs = append(errs, fmt.Errorf("after stop[%d]: %w", i, err))
		}
	}

	return stderrors.Join(errs...)
}

// phaseCtx derives a context for one shutdown phase: the time left in the
// overall budget, floored so a phase running after an earlier overrun still
// gets a chance to finish. The floor never exceeds the configured
// shutdownTimeout, so a caller asking for a tight budget gets one.
func (a *App) phaseCtx(parent context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	floor := min(a.opts.shutdownTimeout, minPhaseTimeout)
	budget := max(time.Until(deadline), floor)
	return context.WithTimeout(parent, budget)
}
