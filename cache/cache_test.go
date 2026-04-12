package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pemako/mk/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryCache_setGet(t *testing.T) {
	err := cache.Init(cache.WithMemory("mem_test"))
	require.NoError(t, err)

	c := cache.Client("mem_test")
	require.NotNil(t, c)

	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "key", "value", time.Minute))

	val, err := c.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "value", val)
}

func TestMemoryCache_miss(t *testing.T) {
	err := cache.Init(cache.WithMemory("mem_miss"))
	require.NoError(t, err)

	c := cache.Client("mem_miss")
	_, err = c.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}

func TestMemoryCache_ttl(t *testing.T) {
	err := cache.Init(cache.WithMemory("mem_ttl"))
	require.NoError(t, err)

	c := cache.Client("mem_ttl")
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

	err = cache.Init(cache.WithRedis("redis_test", mr.Addr()))
	require.NoError(t, err)

	c := cache.Client("redis_test")
	require.NotNil(t, c)

	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "k", "v", time.Minute))

	val, err := c.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", val)
}

func TestCache_del(t *testing.T) {
	err := cache.Init(cache.WithMemory("mem_del"))
	require.NoError(t, err)

	c := cache.Client("mem_del")
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "a", "1", 0))
	require.NoError(t, c.Del(ctx, "a"))
	_, err = c.Get(ctx, "a")
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}

func TestCache_exists(t *testing.T) {
	err := cache.Init(cache.WithMemory("mem_exists"))
	require.NoError(t, err)

	c := cache.Client("mem_exists")
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
