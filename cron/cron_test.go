// cron/cron_test.go
package cron_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pemako/mk/cron"
	"github.com/pemako/mk/lock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_runsJob(t *testing.T) {
	s := cron.New()
	var count atomic.Int32
	require.NoError(t, s.Add("tick", "@every 50ms", func(_ context.Context) error {
		count.Add(1)
		return nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go s.Start(ctx)
	defer s.Stop(context.Background())

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, count.Load(), int32(2))
}

func TestScheduler_retryOnError(t *testing.T) {
	s := cron.New()
	var calls atomic.Int32
	require.NoError(t, s.Add("retry-job", "@every 50ms",
		func(_ context.Context) error {
			if calls.Add(1) < 3 {
				return assert.AnError
			}
			return nil
		},
		cron.WithRetries(3),
		cron.WithRetryDelay(5*time.Millisecond),
	))

	ctx := context.Background()
	go s.Start(ctx)
	defer s.Stop(ctx)

	time.Sleep(400 * time.Millisecond)
	assert.GreaterOrEqual(t, calls.Load(), int32(3))
}

func TestScheduler_skipIfRunning(t *testing.T) {
	s := cron.New()
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	require.NoError(t, s.Add("slow-job", "@every 30ms",
		func(_ context.Context) error {
			n := concurrent.Add(1)
			for {
				old := maxConcurrent.Load()
				if n <= old || maxConcurrent.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(80 * time.Millisecond)
			concurrent.Add(-1)
			return nil
		},
		cron.WithSkipIfRunning(),
	))

	ctx := context.Background()
	go s.Start(ctx)
	defer s.Stop(ctx)

	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, int32(1), maxConcurrent.Load())
}

func TestScheduler_panicRecovery(t *testing.T) {
	s := cron.New()
	var ran atomic.Int32
	require.NoError(t, s.Add("panic-job", "@every 50ms", func(_ context.Context) error {
		ran.Add(1)
		panic("test panic")
	}))

	ctx := context.Background()
	go s.Start(ctx)
	defer s.Stop(ctx)

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, ran.Load(), int32(2), "scheduler must survive panics")
}

func TestScheduler_implementsCoreServer(t *testing.T) {
	s := cron.New()
	var _ interface {
		Start(context.Context) error
		Stop(context.Context) error
	} = s
}

func TestScheduler_Remove(t *testing.T) {
	s := cron.New()
	var count atomic.Int32
	require.NoError(t, s.Add("removable", "@every 30ms", func(_ context.Context) error {
		count.Add(1)
		return nil
	}))

	ctx := context.Background()
	go s.Start(ctx)
	defer s.Stop(ctx)

	time.Sleep(100 * time.Millisecond)
	s.Remove("removable")
	before := count.Load()
	time.Sleep(120 * time.Millisecond)
	assert.Equal(t, before, count.Load(), "job must not run after removal")
}

func TestScheduler_distributedLock_onlyOneRuns(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	locker1 := lock.New(mr.Addr())
	locker2 := lock.New(mr.Addr())

	s1 := cron.New()
	s2 := cron.New()

	var count atomic.Int32
	jobFn := func(_ context.Context) error {
		count.Add(1)
		time.Sleep(60 * time.Millisecond) // hold lock during execution
		return nil
	}

	require.NoError(t, s1.Add("locked-job", "@every 50ms", jobFn,
		cron.WithDistributedLock(locker1, cron.LockTTL(5*time.Second)),
	))
	require.NoError(t, s2.Add("locked-job", "@every 50ms", jobFn,
		cron.WithDistributedLock(locker2, cron.LockTTL(5*time.Second)),
	))

	ctx := context.Background()
	go s1.Start(ctx)
	go s2.Start(ctx)
	defer s1.Stop(ctx)
	defer s2.Stop(ctx)

	time.Sleep(200 * time.Millisecond)
	// With the lock, count should be much less than 2*schedulers*ticks
	// Without the lock it would be ~8. With the lock it should be <= 4.
	assert.LessOrEqual(t, count.Load(), int32(4),
		"distributed lock should prevent most duplicate runs")
}
