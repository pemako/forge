package idgen

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	mu.Lock()
	clear(registry)
	mu.Unlock()
}

func TestInit_DuplicateError(t *testing.T) {
	reset()
	require.NoError(t, Init("gen", WithWorkerID(1)))
	err := Init("gen", WithWorkerID(2))
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestReplace_Succeeds(t *testing.T) {
	reset()
	require.NoError(t, Init("gen", WithWorkerID(1)))
	require.NoError(t, Replace("gen", WithWorkerID(2)))
	require.NotNil(t, Get("gen"))
}
