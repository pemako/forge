// lock/lock_test.go
package lock_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pemako/mk/errors"
	"github.com/pemako/mk/lock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newLocker(t *testing.T) (*lock.Locker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	return lock.New(mr.Addr()), mr
}

func TestAcquire_success(t *testing.T) {
	l, _ := newLocker(t)
	acquired, err := l.Acquire(context.Background(), "key1", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestAcquire_alreadyHeld(t *testing.T) {
	l, _ := newLocker(t)
	ctx := context.Background()
	_, err := l.Acquire(ctx, "key2", 5*time.Second)
	require.NoError(t, err)
	acquired, err := l.Acquire(ctx, "key2", 5*time.Second)
	require.NoError(t, err)
	assert.False(t, acquired)
}

func TestRelease_clearsKey(t *testing.T) {
	l, mr := newLocker(t)
	ctx := context.Background()
	_, err := l.Acquire(ctx, "key3", 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, l.Release(ctx, "key3"))
	val, _ := mr.Get("key3")
	assert.Equal(t, "", val)
}

func TestRelease_doesNotDeleteForeignLock(t *testing.T) {
	l1, mr := newLocker(t)
	l2 := lock.New(mr.Addr())
	ctx := context.Background()
	_, err := l1.Acquire(ctx, "key4", 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, l2.Release(ctx, "key4")) // l2 never acquired it
	val, _ := mr.Get("key4")
	assert.NotEmpty(t, val) // key should still exist
}

func TestTryLock_success(t *testing.T) {
	l, _ := newLocker(t)
	ran := false
	err := l.TryLock(context.Background(), "key5", 5*time.Second, func(_ context.Context) error {
		ran = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, ran)
}

func TestTryLock_notAcquired(t *testing.T) {
	l1, mr := newLocker(t)
	l2 := lock.New(mr.Addr())
	ctx := context.Background()
	_, err := l1.Acquire(ctx, "key6", 5*time.Second)
	require.NoError(t, err)
	err = l2.TryLock(ctx, "key6", 5*time.Second, func(_ context.Context) error { return nil })
	assert.True(t, errors.Is(err, lock.ErrLockNotAcquired))
}

func TestNewWithClient(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	l := lock.NewWithClient(rdb)
	assert.NotNil(t, l)

	ctx := context.Background()
	acquired, err := l.Acquire(ctx, "client-key", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestTryLock_fnError_releasesLock(t *testing.T) {
	l, _ := newLocker(t)
	ctx := context.Background()
	err := l.TryLock(ctx, "fn-err-key", 5*time.Second, func(_ context.Context) error {
		return stderrors.New("fn failed")
	})
	require.Error(t, err)
	assert.Equal(t, "fn failed", err.Error())

	// Lock should be released after fn error
	acquired, err := l.Acquire(ctx, "fn-err-key", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, acquired, "lock should be released after TryLock fn error")
}

func TestAcquire_release_reacquire(t *testing.T) {
	l, _ := newLocker(t)
	ctx := context.Background()

	acquired, err := l.Acquire(ctx, "rr-key", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, acquired)

	require.NoError(t, l.Release(ctx, "rr-key"))

	// Should be acquirable again after release
	acquired2, err := l.Acquire(ctx, "rr-key", 5*time.Second)
	require.NoError(t, err)
	assert.True(t, acquired2)
}

func TestWithWait_options(t *testing.T) {
	opt := lock.WithWait(5*time.Second, 100*time.Millisecond)
	assert.NotNil(t, opt)
}

func TestWithAutoRenew_option(t *testing.T) {
	opt := lock.WithAutoRenew()
	assert.NotNil(t, opt)
}

func TestWithRedisPassword_option(t *testing.T) {
	opt := lock.WithRedisPassword("secret")
	assert.NotNil(t, opt)
}

func TestWithRedisDB_option(t *testing.T) {
	opt := lock.WithRedisDB(1)
	assert.NotNil(t, opt)
}

func TestWithLogger_option(t *testing.T) {
	opt := lock.WithLogger(nil)
	assert.NotNil(t, opt)
}

func TestTryLock_withWait_acquires(t *testing.T) {
	l1, mr := newLocker(t)
	l2 := lock.New(mr.Addr())
	ctx := context.Background()

	// l1 acquires the lock
	acquired, err := l1.Acquire(ctx, "wait-key", 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, acquired)

	// Advance miniredis time so the lock expires
	mr.FastForward(200 * time.Millisecond)

	// l2 tries with wait; l1's lock has expired so l2 should acquire it
	err = l2.TryLock(ctx, "wait-key", 5*time.Second, func(_ context.Context) error {
		return nil
	}, lock.WithWait(2*time.Second, 50*time.Millisecond))
	require.NoError(t, err)
}

func TestTryLock_withAutoRenew(t *testing.T) {
	l, _ := newLocker(t)
	ctx := context.Background()

	err := l.TryLock(ctx, "autorenew-key", 5*time.Second, func(_ context.Context) error {
		// just a quick operation
		return nil
	}, lock.WithAutoRenew())
	require.NoError(t, err)
}
