package cache_test

import (
	"context"
	"testing"

	"github.com/pemako/mk/cache"
	"github.com/stretchr/testify/require"
)

func TestManager_HealthChecker_Memory(t *testing.T) {
	mgr, err := cache.New(cache.WithMemory("default"))
	require.NoError(t, err)
	hc := mgr.HealthChecker("default")
	require.Equal(t, "cache:default", hc.Name())
	require.NoError(t, hc.Check(context.Background()))
}

func TestManager_HealthChecker_Missing(t *testing.T) {
	mgr, err := cache.New(cache.WithMemory("default"))
	require.NoError(t, err)
	hc := mgr.HealthChecker("nonexistent")
	require.Error(t, hc.Check(context.Background()))
}
