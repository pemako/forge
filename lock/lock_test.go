// lock/lock_test.go
package lock_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pemako/mk/errors"
	"github.com/pemako/mk/lock"
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
