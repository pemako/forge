// Package cron provides a named-job cron scheduler that implements core.Server
// for integration with application lifecycle management.
//
// Create a Scheduler with New and register jobs with Add. The scheduler
// supports standard 5-field cron expressions, robfig shorthand specifiers
// (@hourly, @every 5m, etc.), and sub-second @every intervals driven by a
// time.Ticker. Each job can be independently configured with retry, timeout,
// and overlap-prevention options.
//
// Typical usage:
//
//	s := cron.New(cron.WithLocation(time.Local))
//	s.Add("cleanup", "@every 1h", cleanupFn,
//	    cron.WithTimeout(10*time.Minute),
//	    cron.WithSkipIfRunning(),
//	)
//	app.WithServer(s)  // integrates with graceful shutdown
package cron

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/lock"
	"github.com/pemako/forge/logger"
)

// --- Scheduler-level options ---

type schedulerOptions struct {
	log      core.Logger
	location *time.Location
}

// Option configures the Scheduler.
type Option func(*schedulerOptions)

// WithLogger sets the logger for job lifecycle events (start, done, skip, failed).
// Defaults to logger.Default().
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
	distLock      *distLockOpts // new field
}

// distLockOpts holds distributed lock configuration for a job.
type distLockOpts struct {
	locker *lock.Locker
	ttl    time.Duration // 0 = derive from WithTimeout, else default 30s
	key    string        // 0 = "cron:<job-name>"
}

// LockOption configures the distributed lock for a cron job.
type LockOption func(*distLockOpts)

// LockTTL sets an explicit TTL for the distributed lock.
// If not set, TTL is derived from WithTimeout (if configured) or defaults to 30s.
// TTL must be >= the maximum expected job execution time to avoid premature
// lock expiry and duplicate execution.
func LockTTL(d time.Duration) LockOption {
	return func(o *distLockOpts) { o.ttl = d }
}

// LockKey sets a custom Redis key for the distributed lock.
// Defaults to "cron:<job-name>" when not set.
func LockKey(key string) LockOption {
	return func(o *distLockOpts) { o.key = key }
}

// WithDistributedLock ensures only one instance across the cluster executes
// the job at a time by acquiring a Redis distributed lock before each run.
//
// If the lock is already held by another instance, this instance skips the
// current tick and logs a debug message. The lock is released automatically
// when the job finishes (or panics).
//
// TTL priority:
//  1. LockTTL(d) option
//  2. WithTimeout(d) value (if configured on the same job)
//  3. Default: 30s
//
// Example:
//
//	locker := lock.New("redis:6379")
//	scheduler.Add("send-report", "@every 1h", fn,
//	    cron.WithTimeout(5*time.Minute),
//	    cron.WithDistributedLock(locker),            // TTL auto = 5min
//	)
//	scheduler.Add("cleanup", "@every 5m", fn,
//	    cron.WithDistributedLock(locker,
//	        cron.LockTTL(3*time.Minute),             // explicit TTL
//	        cron.LockKey("my-custom-lock-key"),      // optional key override
//	    ),
//	)
func WithDistributedLock(locker *lock.Locker, opts ...LockOption) JobOption {
	return func(o *jobOptions) {
		dl := &distLockOpts{locker: locker}
		for _, opt := range opts {
			opt(dl)
		}
		o.distLock = dl
	}
}

// JobOption configures a single cron job.
type JobOption func(*jobOptions)

// WithRetries sets the maximum number of retry attempts when a job returns an
// error (default 0 = no retry). Successive retries use exponential backoff
// starting from WithRetryDelay, capped at 30 s.
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
