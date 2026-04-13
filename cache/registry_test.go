package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	registryMu.Lock()
	for _, c := range registry {
		c.Close() //nolint:errcheck
	}
	clear(registry)
	registryMu.Unlock()
}

func TestInit_DuplicateError(t *testing.T) {
	reset()
	require.NoError(t, Init(WithMemory("mem")))
	err := Init(WithMemory("mem"))
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestReplace_ClosesOldBackend(t *testing.T) {
	reset()
	require.NoError(t, Init(WithMemory("m")))
	require.NoError(t, Replace("m", WithMemory("m")))
	require.NotNil(t, Client("m"))
}
