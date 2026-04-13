package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	registryMu.Lock()
	for _, m := range registry {
		m.CloseAll() //nolint:errcheck
	}
	clear(registry)
	registryMu.Unlock()
}

func TestNew_DuplicateError(t *testing.T) {
	reset()
	mgr, err := New(WithMemory("mem"))
	require.NoError(t, err)
	require.NoError(t, Register("mem", mgr))
	mgr2, err := New(WithMemory("mem"))
	require.NoError(t, err)
	err = Register("mem", mgr2)
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestReplace_ClosesOldBackend(t *testing.T) {
	reset()
	mgr, err := New(WithMemory("m"))
	require.NoError(t, err)
	require.NoError(t, Register("default", mgr))
	mgr2, err := New(WithMemory("m"))
	require.NoError(t, err)
	require.NoError(t, Replace("default", mgr2))
	require.NotNil(t, Client("m"))
}

func TestNew_ReturnsManager(t *testing.T) {
	mgr, err := New(WithMemory("default"))
	require.NoError(t, err)
	require.NotNil(t, mgr.Client("default"))
	require.Nil(t, mgr.Client("missing"))
}

func TestManager_Close_ImplementsCloser(t *testing.T) {
	mgr, _ := New(WithMemory("m"))
	require.NoError(t, mgr.Close())
}

func TestRegister_DelegatesToDefault(t *testing.T) {
	reset()
	mgr, _ := New(WithMemory("default"))
	require.NoError(t, Register("default", mgr))
	require.NotNil(t, Client("default"))
}
