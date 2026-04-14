// Package lock/options.go
// Constructor options (Option) and per-operation options (TryLockOption).
package lock

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
)

// --- Constructor options (used with New) ---

type newOptions struct {
	password string
	db       int
	log      core.Logger
}

// Option configures the Locker constructor.
type Option func(*newOptions)

// WithRedisPassword sets the Redis AUTH password for lock.New connections.
func WithRedisPassword(password string) Option {
	return func(o *newOptions) { o.password = password }
}

// WithRedisDB selects the Redis logical database for lock.New connections (default 0).
func WithRedisDB(db int) Option {
	return func(o *newOptions) { o.db = db }
}

// WithLogger sets the logger for lock acquisition, release, and renewal events.
func WithLogger(l core.Logger) Option {
	return func(o *newOptions) { o.log = l }
}

func newOptionsDefaults(opts []Option) newOptions {
	o := newOptions{log: logger.Default()}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithConfig reads lock settings from core.Config.
// Keys: lock.password, lock.db.
// Used together with NewFromConfig which reads lock.addr.
func WithConfig(cfg core.Config) Option {
	return func(o *newOptions) {
		if v := cfg.GetString("lock.password"); v != "" {
			o.password = v
		}
		if v := cfg.GetInt("lock.db"); v != 0 {
			o.db = v
		}
	}
}

// --- Per-operation options (used with TryLock) ---

type tryLockOptions struct {
	waitTimeout  time.Duration
	waitInterval time.Duration
	autoRenew    bool
}

// TryLockOption configures a single TryLock call.
type TryLockOption func(*tryLockOptions)

// WithWait makes TryLock poll for the lock until the timeout elapses or the
// lock is acquired. interval controls how often to retry (e.g., 200ms).
// Without WithWait, TryLock returns ErrLockNotAcquired immediately if the lock
// is held by another caller.
func WithWait(timeout, interval time.Duration) TryLockOption {
	return func(o *tryLockOptions) {
		o.waitTimeout = timeout
		o.waitInterval = interval
	}
}

// WithAutoRenew starts a background goroutine that renews the lock TTL every ttl/3.
// If renewal detects the lock was lost, the context passed to TryLock's fn is
// cancelled and TryLock returns ErrLockExpired.
func WithAutoRenew() TryLockOption {
	return func(o *tryLockOptions) { o.autoRenew = true }
}
