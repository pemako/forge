package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/pemako/forge/ratelimit"
	"github.com/pemako/forge/testutil"
)

func TestInProcess_allow(t *testing.T) {
	lim := ratelimit.New(ratelimit.WithRate(10), ratelimit.WithBurst(10))
	ctx := context.Background()
	for range 10 {
		assert.True(t, lim.Allow(ctx, "user-1"), "should allow within burst")
	}
}

func TestInProcess_deny(t *testing.T) {
	lim := ratelimit.New(ratelimit.WithRate(1), ratelimit.WithBurst(1))
	ctx := context.Background()
	assert.True(t, lim.Allow(ctx, "user-2"))
	assert.False(t, lim.Allow(ctx, "user-2"), "second immediate request should be denied")
}

func TestInProcess_perKey(t *testing.T) {
	lim := ratelimit.New(ratelimit.WithRate(1), ratelimit.WithBurst(1))
	ctx := context.Background()
	assert.True(t, lim.Allow(ctx, "user-a"))
	assert.False(t, lim.Allow(ctx, "user-a"))
	assert.True(t, lim.Allow(ctx, "user-b"), "different key should have its own bucket")
}

func TestInProcess_withConfig(t *testing.T) {
	cfg := testutil.NewConfig(map[string]string{
		"ratelimit.rate":  "5",
		"ratelimit.burst": "5",
	})
	lim := ratelimit.New(ratelimit.WithConfig(cfg))
	ctx := context.Background()
	for range 5 {
		assert.True(t, lim.Allow(ctx, "key"))
	}
	assert.False(t, lim.Allow(ctx, "key"))
}

func TestDistributed_allow(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	lim := ratelimit.New(
		ratelimit.WithRate(5),
		ratelimit.WithWindow(time.Second),
		ratelimit.WithRedis(rdb),
	)
	ctx := context.Background()
	for range 5 {
		assert.True(t, lim.Allow(ctx, "dist-key"))
	}
	assert.False(t, lim.Allow(ctx, "dist-key"), "6th request should be denied")
}

func TestDistributed_windowExpiry(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	lim := ratelimit.New(
		ratelimit.WithRate(2),
		ratelimit.WithWindow(time.Second),
		ratelimit.WithRedis(rdb),
	)
	ctx := context.Background()
	assert.True(t, lim.Allow(ctx, "win-key"))
	assert.True(t, lim.Allow(ctx, "win-key"))
	assert.False(t, lim.Allow(ctx, "win-key"))

	// Advance miniredis clock past the window.
	mr.FastForward(2 * time.Second)

	assert.True(t, lim.Allow(ctx, "win-key"), "should allow after window expiry")
}

func TestDistributed_perKey(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	lim := ratelimit.New(
		ratelimit.WithRate(1),
		ratelimit.WithWindow(time.Second),
		ratelimit.WithRedis(rdb),
	)
	ctx := context.Background()
	assert.True(t, lim.Allow(ctx, "k1"))
	assert.False(t, lim.Allow(ctx, "k1"))
	assert.True(t, lim.Allow(ctx, "k2"), "different key should not be affected")
}
