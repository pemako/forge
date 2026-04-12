package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
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

// Run starts the application lifecycle and blocks until the process receives
// SIGTERM or SIGINT, or until a server returns a fatal error.
// The lifecycle phases executed in order are:
//  1. Migrations (WithMigration) — Up is called once.
//  2. BeforeStart hooks — called sequentially; first error aborts startup.
//  3. All registered Servers are started concurrently.
//  4. Run blocks until SIGTERM/SIGINT or a server error.
//  5. All Servers are stopped concurrently within ShutdownTimeout.
//  6. AfterStop hooks — called sequentially after all servers have stopped.
//
// Run returns a non-nil error if any phase fails; the first error encountered
// is returned. AfterStop errors are only returned if no earlier error occurred.
func (a *App) Run() error {
	ctx := context.Background()

	if a.opts.migrator != nil {
		if err := a.opts.migrator.Up(ctx); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}

	for _, fn := range a.opts.beforeStart {
		if err := fn(ctx); err != nil {
			return fmt.Errorf("before start: %w", err)
		}
	}

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
	}

	stopCtx, cancel := context.WithTimeout(ctx, a.opts.shutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, srv := range a.opts.servers {
		wg.Add(1)
		srv := srv
		go func() {
			defer wg.Done()
			srv.Stop(stopCtx)
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-stopCtx.Done():
	}

	// Wait for all Start goroutines to finish so callers see consistent state.
	startWg.Wait()

	for _, fn := range a.opts.afterStop {
		if err := fn(stopCtx); err != nil && runErr == nil {
			runErr = fmt.Errorf("after stop: %w", err)
		}
	}

	return runErr
}
