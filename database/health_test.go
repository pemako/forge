package database_test

import (
	"context"
	"testing"

	"github.com/pemako/mk/database"
	"github.com/stretchr/testify/require"
)

func TestManager_HealthChecker(t *testing.T) {
	mgr, err := database.New(database.WithDriver("main", "sqlite", "file::memory:"))
	require.NoError(t, err)
	hc := mgr.HealthChecker("main")
	require.Equal(t, "database:main", hc.Name())
	require.NoError(t, hc.Check(context.Background()))
}

func TestManager_HealthChecker_UnknownConn(t *testing.T) {
	mgr, err := database.New(database.WithDriver("main", "sqlite", "file::memory:"))
	require.NoError(t, err)
	hc := mgr.HealthChecker("missing")
	require.Error(t, hc.Check(context.Background()))
}
