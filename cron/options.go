// cron/options.go
package cron

import (
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

// --- Scheduler-level options ---

type schedulerOptions struct {
	log      core.Logger
	location *time.Location
}

// Option configures the Scheduler.
type Option func(*schedulerOptions)

// WithLogger sets the logger for job lifecycle events. Defaults to logger.Default().
func WithLogger(l core.Logger) Option {
	return func(o *schedulerOptions) { o.log = l }
}

// WithLocation sets the timezone for cron expression evaluation. Defaults to time.UTC.
func WithLocation(loc *time.Location) Option {
	return func(o *schedulerOptions) { o.location = loc }
}

func defaultSchedulerOptions() schedulerOptions {
	return schedulerOptions{
		log:      logger.Default(),
		location: time.UTC,
	}
}

// --- Per-job options ---

type jobOptions struct {
	retries       int
	retryDelay    time.Duration
	timeout       time.Duration
	skipIfRunning bool
}

// JobOption configures a single cron job.
type JobOption func(*jobOptions)

// WithRetries sets the maximum retry attempts on failure (default 0 = no retry).
// Retries use exponential backoff starting from the retry delay.
func WithRetries(n int) JobOption {
	return func(o *jobOptions) { o.retries = n }
}

// WithRetryDelay sets the base delay between retry attempts (default 1s).
func WithRetryDelay(d time.Duration) JobOption {
	return func(o *jobOptions) { o.retryDelay = d }
}

// WithTimeout sets a deadline for each job execution. The context passed to
// the job function is cancelled when the timeout is reached.
func WithTimeout(d time.Duration) JobOption {
	return func(o *jobOptions) { o.timeout = d }
}

// WithSkipIfRunning prevents overlapping executions. If the job is still running
// when the next tick fires, the new execution is skipped.
func WithSkipIfRunning() JobOption {
	return func(o *jobOptions) { o.skipIfRunning = true }
}

func defaultJobOptions() jobOptions {
	return jobOptions{retryDelay: time.Second}
}
