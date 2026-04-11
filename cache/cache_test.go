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

func TestCache_setGet(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	err = cache.Init(cache.WithAddr("cache_default", mr.Addr()))
	require.NoError(t, err)

	rdb := cache.Client("cache_default")
	require.NotNil(t, rdb)

	ctx := context.Background()
	require.NoError(t, rdb.Set(ctx, "k", "v", time.Minute).Err())

	val, err := rdb.Get(ctx, "k").Result()
	require.NoError(t, err)
	assert.Equal(t, "v", val)
}

func TestCache_unknownClient(t *testing.T) {
	assert.Nil(t, cache.Client("nonexistent_cache_xyz"))
}
