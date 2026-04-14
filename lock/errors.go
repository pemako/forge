// Package lock provides distributed mutual exclusion backed by Redis.
//
// Create a Locker with New (manages its own Redis connection) or NewWithClient
// (shares an existing *redis.Client). Use Acquire/Release for manual control,
// or TryLock for the acquire-execute-release pattern with optional wait polling
// and automatic TTL renewal.
//
// Typical usage:
//
//	locker := lock.New("redis:6379")
//	err := locker.TryLock(ctx, "job:process", 30*time.Second, func(ctx context.Context) error {
//	    return doWork(ctx)
//	}, lock.WithAutoRenew())
//	if errors.Is(err, lock.ErrLockNotAcquired) {
//	    // another instance holds the lock — skip
//	}
package lock

import mkerrors "github.com/pemako/forge/errors"

// ErrLockNotAcquired is returned by TryLock when the lock key is already held
// by another caller and no WithWait option was supplied (or the wait timed out).
// This is a normal business condition, not a system error.
var ErrLockNotAcquired = mkerrors.Define(409, "LOCK_NOT_ACQUIRED", "lock is held by another instance")

// ErrLockExpired is returned by TryLock when the lock is lost mid-execution
// because auto-renewal detected another instance acquired the key after TTL
// expiry. The context passed to fn will have been cancelled before this error
// is returned.
var ErrLockExpired = mkerrors.Define(500, "LOCK_EXPIRED", "lock expired during execution")
