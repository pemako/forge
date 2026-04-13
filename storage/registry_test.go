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
	// S3 with only bucket+region — AWS SDK allows construction without real credentials.
	err1 := Register("s3test",
		WithProvider("s3"),
		WithBucket("b1"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.NoError(t, err1)
	err2 := Register("s3test",
		WithProvider("s3"),
		WithBucket("b2"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	)
	require.ErrorIs(t, err2, ErrAlreadyRegistered)
}

func TestReplace_Succeeds(t *testing.T) {
	reset()
	require.NoError(t, Register("r",
		WithProvider("s3"),
		WithBucket("b1"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	))
	require.NoError(t, Replace("r",
		WithProvider("s3"),
		WithBucket("b2"),
		WithRegion("us-east-1"),
		WithAccessKey("k", "s"),
	))
	require.NotNil(t, Get("r"))
}
