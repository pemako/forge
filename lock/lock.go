// Package lock/lock.go
// Locker implementation: Acquire, Release, and TryLock.
package lock

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/pemako/mk/core"
)

const (
	// acquireLua sets key=token only if key does not exist, with TTL in milliseconds.
	acquireLua = `return redis.call("SET", KEYS[1], ARGV[1], "NX", "PX", ARGV[2])`

	// releaseLua deletes key only if the stored value matches token (prevents
	// releasing a lock acquired by another instance).
	releaseLua = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end`
)

// Locker provides distributed mutual exclusion backed by Redis.
// Lock acquisition and release use Lua scripts for atomicity, ensuring that
// only the instance that acquired a lock can release it.
// Create with New or NewWithClient; all methods are safe for concurrent use.
type Locker struct {
	client *redis.Client
	log    core.Logger
	tokens sync.Map // key → token for this Locker instance
}

// New creates a Locker that opens its own Redis connection to addr.
// Use NewWithClient when sharing a pre-existing *redis.Client.
func New(addr string, opts ...Option) *Locker {
	o := newOptionsDefaults(opts)
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: o.password,
		DB:       o.db,
	})
	return &Locker{client: rdb, log: o.log}
}

// NewWithClient creates a Locker using an existing *redis.Client.
// The caller retains ownership of the client's lifecycle.
func NewWithClient(rdb *redis.Client, opts ...Option) *Locker {
	o := newOptionsDefaults(opts)
	return &Locker{client: rdb, log: o.log}
}

// Acquire attempts to set key in Redis with a unique token and the given TTL.
// Returns (true, nil) if the lock was acquired, (false, nil) if the key is
// already held by another caller, or (false, err) on a Redis error.
// See TryLock for an acquire-execute-release convenience wrapper.
func (l *Locker) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	token := uuid.NewString()
	result, err := l.client.Eval(ctx, acquireLua, []string{key}, token, ttl.Milliseconds()).Text()
	if err != nil && err != redis.Nil {
		return false, err
	}
	if result == "OK" {
		l.tokens.Store(key, token)
		l.log.Debug(ctx, "lock acquired", core.Field{Key: "key", Value: key})
		return true, nil
	}
	return false, nil
}

// Release deletes the lock for key from Redis, but only if this Locker holds
// it (i.e. the stored token matches). It is a no-op if this Locker never
// acquired key, preventing accidental release of a lock owned by another
// instance after a TTL expiry.
func (l *Locker) Release(ctx context.Context, key string) error {
	val, ok := l.tokens.Load(key)
	if !ok {
		return nil
	}
	token := val.(string)
	l.tokens.Delete(key)
	_, err := l.client.Eval(ctx, releaseLua, []string{key}, token).Int()
	if err != nil && err != redis.Nil {
		return err
	}
	l.log.Debug(ctx, "lock released", core.Field{Key: "key", Value: key})
	return nil
}

// TryLock acquires the lock, runs fn with a context, then releases the lock.
// If the lock is unavailable and WithWait is not set, returns ErrLockNotAcquired.
// If WithAutoRenew is set and the lock is lost mid-execution, the context passed
// to fn is cancelled and TryLock returns ErrLockExpired.
func (l *Locker) TryLock(ctx context.Context, key string, ttl time.Duration, fn func(context.Context) error, opts ...TryLockOption) error {
	to := tryLockOptions{waitInterval: 100 * time.Millisecond}
	for _, opt := range opts {
		opt(&to)
	}

	acquired, err := l.acquireWithWait(ctx, key, ttl, to)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrLockNotAcquired
	}
	defer l.Release(context.Background(), key)

	execCtx := ctx
	if to.autoRenew {
		val, _ := l.tokens.Load(key)
		token := val.(string)
		renewer, lockCtx := newAutoRenewer(l.client, key, token, ttl, l.log)
		renewer.start()
		defer renewer.stopRenew()
		execCtx = lockCtx
	}

	if err := fn(execCtx); err != nil {
		return err
	}
	if execCtx.Err() != nil {
		return ErrLockExpired
	}
	return nil
}

func (l *Locker) acquireWithWait(ctx context.Context, key string, ttl time.Duration, to tryLockOptions) (bool, error) {
	acquired, err := l.Acquire(ctx, key, ttl)
	if err != nil || acquired || to.waitTimeout == 0 {
		return acquired, err
	}

	deadline := time.Now().Add(to.waitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(to.waitInterval):
		}
		acquired, err = l.Acquire(ctx, key, ttl)
		if err != nil || acquired {
			return acquired, err
		}
	}
	return false, nil
}
