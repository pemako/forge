// Package cron/job.go
// Job execution helpers: panic recovery, timeout wrapping, and retry backoff.
package cron

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/pemako/mk/core"
)

// executeJob wraps a job function with recover, optional timeout, and retry.
// Logs start/done/skip/failed events via log.
func executeJob(ctx context.Context, name string, fn func(context.Context) error, opts jobOptions, running *atomic.Int32, log core.Logger) {
	if opts.skipIfRunning && !running.CompareAndSwap(0, 1) {
		log.Warn(ctx, "cron skip",
			core.Field{Key: "job", Value: name},
			core.Field{Key: "reason", Value: "still running"},
		)
		return
	}
	if opts.skipIfRunning {
		defer running.Store(0)
	}

	log.Info(ctx, "cron start", core.Field{Key: "job", Value: name})
	start := time.Now()

	for attempt := range opts.retries + 1 {
		if attempt > 0 {
			delay := cronBackoff(attempt, opts.retryDelay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}

		err := runOnce(ctx, fn, opts.timeout)
		if err == nil {
			log.Info(ctx, "cron done",
				core.Field{Key: "job", Value: name},
				core.Field{Key: "elapsed", Value: time.Since(start).String()},
			)
			return
		}
		log.Error(ctx, "cron failed",
			core.Field{Key: "job", Value: name},
			core.Field{Key: "err", Value: err.Error()},
			core.Field{Key: "retry", Value: attempt},
		)
	}
}

// runOnce executes fn once with panic recovery and optional timeout.
func runOnce(ctx context.Context, fn func(context.Context) error, timeout time.Duration) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()

	execCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return fn(execCtx)
}

// cronBackoff returns the wait before retry attempt n (1-based), exponential
// backoff capped at 30s.
func cronBackoff(attempt int, base time.Duration) time.Duration {
	const maxDelay = 30 * time.Second
	shift := min(attempt-1, 5)
	d := base * (1 << shift)
	if d > maxDelay {
		return maxDelay
	}
	return d
}
