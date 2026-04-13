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

func TestNew_DuplicateError(t *testing.T) {
	reset()
	g1, err := New(WithWorkerID(1))
	require.NoError(t, err)
	require.NoError(t, Register("gen", g1))

	g2, err := New(WithWorkerID(2))
	require.NoError(t, err)
	err = Register("gen", g2)
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestReplace_Succeeds(t *testing.T) {
	reset()
	g1, _ := New(WithWorkerID(1))
	require.NoError(t, Register("gen", g1))
	g2, _ := New(WithWorkerID(2))
	require.NoError(t, Replace("gen", g2))
	require.NotNil(t, Get("gen"))
}
