package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	registryMu.Lock()
	clear(registry)
	registryMu.Unlock()
}

func TestNew_DoesNotRegisterGlobally(t *testing.T) {
	reset()
	_, err := New(WithDriver("db1", "sqlite", "file::memory:"))
	require.NoError(t, err)
	require.Nil(t, Get("anything")) // registry not touched
}

func TestManager_DB_ReturnsConnection(t *testing.T) {
	mgr, err := New(WithDriver("main", "sqlite", "file::memory:"))
	require.NoError(t, err)
	require.NotNil(t, mgr.DB("main"))
	require.Nil(t, mgr.DB("nonexistent"))
}

func TestRegister_StoresManager(t *testing.T) {
	reset()
	mgr, _ := New(WithDriver("main", "sqlite", "file::memory:"))
	require.NoError(t, Register("default", mgr))
	require.Equal(t, mgr, Get("default"))
}

func TestRegister_DuplicateError(t *testing.T) {
	reset()
	mgr, _ := New(WithDriver("main", "sqlite", "file::memory:"))
	require.NoError(t, Register("default", mgr))
	err := Register("default", mgr)
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestDB_ConvenienceDelegatesToDefault(t *testing.T) {
	reset()
	mgr, _ := New(WithDriver("main", "sqlite", "file::memory:"))
	Register("default", mgr) //nolint:errcheck
	require.NotNil(t, DB("main"))
	require.Nil(t, DB("unknown"))
}

func TestCloseAll_ClosesManagers(t *testing.T) {
	reset()
	mgr1, _ := New(WithDriver("db1", "sqlite", "file::memory:"))
	mgr2, _ := New(WithDriver("db2", "sqlite", "file::memory:"))
	require.NoError(t, Register("a", mgr1))
	require.NoError(t, Register("b", mgr2))
	require.NoError(t, CloseAll())
	require.Nil(t, Get("a"))
	require.Nil(t, Get("b"))
}
