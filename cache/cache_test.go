package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pemako/forge/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMgr(t *testing.T, opts ...cache.Option) *cache.Manager {
	t.Helper()
	mgr, err := cache.New(opts...)
	require.NoError(t, err)
	return mgr
}

func TestMemoryCache_setGet(t *testing.T) {
	mgr := newMgr(t, cache.WithMemory("mem_test"))
	c := mgr.Client("mem_test")
	require.NotNil(t, c)

	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "key", "value", time.Minute))

	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestMemoryCache_miss(t *testing.T) {
	mgr := newMgr(t, cache.WithMemory("mem_miss"))
	c := mgr.Client("mem_miss")
	_, err := c.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}

func TestMemoryCache_ttl(t *testing.T) {
	mgr := newMgr(t, cache.WithMemory("mem_ttl"))
	c := mgr.Client("mem_ttl")
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "expiring", "val", 30*time.Millisecond))

	val, err := c.Get(ctx, "expiring")
	require.NoError(t, err)
	assert.Equal(t, "val", val)

	time.Sleep(50 * time.Millisecond)
	_, err = c.Get(ctx, "expiring")
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}

func TestRedisCache_setGet(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	mgr := newMgr(t, cache.WithRedis("redis_test", mr.Addr()))
	c := mgr.Client("redis_test")
	require.NotNil(t, c)

	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "k", "v", time.Minute))

	val, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", val)
}

func TestCache_del(t *testing.T) {
	mgr := newMgr(t, cache.WithMemory("mem_del"))
	c := mgr.Client("mem_del")
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "a", "1", 0))
	require.NoError(t, c.Del(ctx, "a"))
	_, err := c.Get(ctx, "a")
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}

func TestCache_exists(t *testing.T) {
	mgr := newMgr(t, cache.WithMemory("mem_exists"))
	c := mgr.Client("mem_exists")
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "x", "1", 0))

	ok, err := c.Exists(ctx, "x")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = c.Exists(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClient_unknownName(t *testing.T) {
	assert.Nil(t, cache.Client("nonexistent_xyz_abc"))
}
