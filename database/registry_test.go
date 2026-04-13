package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	registryMu.Lock()
	for _, db := range registry {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close() //nolint:errcheck
		}
	}
	clear(registry)
	registryMu.Unlock()
}

func TestNew_DuplicateConnectionError(t *testing.T) {
	reset()
	_, err := New(WithDriver("dup", "sqlite", "file::memory:"))
	require.NoError(t, err)
	_, err = New(WithDriver("dup", "sqlite", "file::memory:"))
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}
