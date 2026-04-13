package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func reset() {
	registryMu.Lock()
	clear(registry)
	registryMu.Unlock()
}

func TestRegister_DuplicateError(t *testing.T) {
	reset()
	s1, err := New(
		WithProvider("s3"),
		WithBucket("b1"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.NoError(t, err)
	require.NoError(t, Register("s3test", s1))

	s2, err := New(
		WithProvider("s3"),
		WithBucket("b2"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.NoError(t, err)
	err = Register("s3test", s2)
	require.ErrorIs(t, err, ErrAlreadyRegistered)
}

func TestReplace_Succeeds(t *testing.T) {
	reset()
	s1, err := New(
		WithProvider("s3"),
		WithBucket("b1"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.NoError(t, err)
	require.NoError(t, Register("r", s1))

	s2, err := New(
		WithProvider("s3"),
		WithBucket("b2"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.NoError(t, err)
	require.NoError(t, Replace("r", s2))
	require.NotNil(t, Get("r"))
}

func TestCloseAll_ClearsRegistry(t *testing.T) {
	reset()
	s, err := New(
		WithProvider("s3"),
		WithBucket("b1"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.NoError(t, err)
	require.NoError(t, Register("ca", s))
	require.NotNil(t, Get("ca"))
	require.NoError(t, CloseAll())
	require.Nil(t, Get("ca"))
}
