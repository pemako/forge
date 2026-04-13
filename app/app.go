package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/pemako/mk/core"
)

// App is the application lifecycle manager. Create one with New, then call Run.
type App struct {
	opts options
}

// New creates an App with the given options. The following defaults are applied
// when options are not provided: name="app", version="unknown",
// shutdownTimeout=30s.
func New(opts ...Option) *App {
	o := options{
		name:            "app",
		version:         "unknown",
		shutdownTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &App{opts: o}
}

// log emits a structured Info log if a logger is configured.
func (a *App) log(ctx context.Context, msg string, fields ...core.Field) {
	if a.opts.logger != nil {
		a.opts.logger.Info(ctx, msg, fields...)
	}
}

// HealthCheckers returns the registered health checkers.
// Use in a WithBeforeStart hook to wire them into server/http:
//
//	app.WithBeforeStart(func(ctx context.Context) error {
//	    for _, hc := range myApp.HealthCheckers() {
//	        httpSrv.AddReadyCheck(hc.Name(), hc.Check)
//	    }
//	    return nil
//	})
func (a *App) HealthCheckers() []core.HealthChecker {
	return a.opts.checkers
}

// Run starts the application lifecycle and blocks until the process receives
// SIGTERM or SIGINT, or until a server returns a fatal error.
// The lifecycle phases executed in order are:
//  1. Migrations (WithMigration) — Up is called once.
//  2. Lifecycle.Init() — sequential; on failure, already-init'd components are
//     Shutdown in reverse order before Run returns.
//  3. BeforeStart hooks — called sequentially; first error aborts startup.
//  4. All registered Servers are started concurrently.
//  5. Run blocks until SIGTERM/SIGINT or a server error.
//  6. All Servers are stopped concurrently within ShutdownTimeout; each stop
//     error is logged and the first becomes the return error if none prior.
//  7. Lifecycle.Shutdown() — reverse registration order.
//  8. Closer.Close() — registration order; errors collected but do not stop subsequent Closers.
//  9. AfterStop hooks — called sequentially after all of the above.
//
// Run returns a non-nil error if any phase fails; the first error encountered
// is returned.
func (a *App) Run() error {
	ctx := context.Background()

	if a.opts.migrator != nil {
		if err := a.opts.migrator.Up(ctx); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
		a.log(ctx, "app: migrations complete")
	}

	for i, lc := range a.opts.lifecycles {
		if err := lc.Init(ctx); err != nil {
			for j := i - 1; j >= 0; j-- {
				if shutErr := a.opts.lifecycles[j].Shutdown(ctx); shutErr != nil {
					a.log(ctx, "app: lifecycle rollback shutdown error",
						core.Field{Key: "index", Value: j},
						core.Field{Key: "err", Value: shutErr.Error()},
					)
				}
			}
			return fmt.Errorf("lifecycle init[%d]: %w", i, err)
		}
	}

	for _, fn := range a.opts.beforeStart {
		if err := fn(ctx); err != nil {
			return fmt.Errorf("before start: %w", err)
		}
	}

	a.log(ctx, "app: starting servers", core.Field{Key: "count", Value: len(a.opts.servers)})

	errCh := make(chan error, len(a.opts.servers)+1)
	var startWg sync.WaitGroup
	for _, srv := range a.opts.servers {
		startWg.Add(1)
		srv := srv
		go func() {
			defer startWg.Done()
			if err := srv.Start(ctx); err != nil {
				errCh <- err
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(quit)

	var runErr error
	select {
	case err := <-errCh:
		runErr = fmt.Errorf("server error: %w", err)
	case <-quit:
		a.log(ctx, "app: shutdown signal received")
	}

	stopCtx, cancel := context.WithTimeout(ctx, a.opts.shutdownTimeout)
	defer cancel()

	stopErrs := make([]error, len(a.opts.servers))
	var stopWg sync.WaitGroup
	for i, srv := range a.opts.servers {
		stopWg.Add(1)
		i, srv := i, srv
		go func() {
			defer stopWg.Done()
			if err := srv.Stop(stopCtx); err != nil {
				stopErrs[i] = err
				a.log(ctx, "app: server stop error", core.Field{Key: "err", Value: err.Error()})
			}
		}()
	}
	stopDone := make(chan struct{})
	go func() { stopWg.Wait(); close(stopDone) }()
	select {
	case <-stopDone:
	case <-stopCtx.Done():
		a.log(ctx, "app: shutdown timeout exceeded")
	}

	// Wait for Start goroutines to exit, but respect the shutdown deadline.
	// If a server's Start is hung (e.g., blocked on I/O), we log and move on
	// rather than blocking forever.
	startDone := make(chan struct{})
	go func() { startWg.Wait(); close(startDone) }()
	select {
	case <-startDone:
	case <-stopCtx.Done():
		a.log(ctx, "app: start goroutines did not exit within shutdown timeout")
	}

	var stopJoined []error
	for _, err := range stopErrs {
		if err != nil {
			stopJoined = append(stopJoined, fmt.Errorf("server stop: %w", err))
		}
	}
	if len(stopJoined) > 0 {
		joined := errors.Join(stopJoined...)
		if runErr == nil {
			runErr = joined
		} else {
			runErr = errors.Join(runErr, joined)
		}
	}

	for i := len(a.opts.lifecycles) - 1; i >= 0; i-- {
		if err := a.opts.lifecycles[i].Shutdown(stopCtx); err != nil {
			a.log(ctx, "app: lifecycle shutdown error",
				core.Field{Key: "index", Value: i},
				core.Field{Key: "err", Value: err.Error()},
			)
			if runErr == nil {
				runErr = fmt.Errorf("lifecycle shutdown[%d]: %w", i, err)
			}
		}
	}

	for _, c := range a.opts.closers {
		if err := c.Close(); err != nil {
			a.log(ctx, "app: closer error", core.Field{Key: "err", Value: err.Error()})
			if runErr == nil {
				runErr = fmt.Errorf("closer: %w", err)
			}
		}
	}

	for _, fn := range a.opts.afterStop {
		if err := fn(stopCtx); err != nil && runErr == nil {
			runErr = fmt.Errorf("after stop: %w", err)
		}
	}

	a.log(ctx, "app: shutdown complete")
	return runErr
}
