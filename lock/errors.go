// lock/errors.go
package lock

import mkerrors "github.com/pemako/mk/errors"

// ErrLockNotAcquired is returned when the lock is already held by another caller.
// This is a normal business condition, not a system error.
var ErrLockNotAcquired = mkerrors.Define(409, "LOCK_NOT_ACQUIRED", "lock is held by another instance")

// ErrLockExpired is returned by TryLock when the lock is lost mid-execution
// because auto-renewal detected another instance acquired the key after TTL expiry.
var ErrLockExpired = mkerrors.Define(500, "LOCK_EXPIRED", "lock expired during execution")
