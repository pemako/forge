package storage_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pemako/mk/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_unknownProvider(t *testing.T) {
	_, err := storage.New(storage.WithProvider("unknown"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestNew_s3_missingBucket(t *testing.T) {
	_, err := storage.New(storage.WithProvider("s3"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestWithContentType(t *testing.T) {
	// UploadOption wiring test — no real backend needed
	_ = strings.NewReader("data")
	assert.NotNil(t, storage.WithContentType("image/png"))
}

func TestRegistry_GetNil(t *testing.T) {
	s := storage.Get("nonexistent_storage_xyz")
	assert.Nil(t, s)
}

func TestWithMetadata(t *testing.T) {
	assert.NotNil(t, storage.WithMetadata("key", "value"))
}

func TestNew_oss_missingBucketOrEndpoint(t *testing.T) {
	_, err := storage.New(storage.WithProvider("oss"), storage.WithBucket("my-bucket"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint")
}

func TestNew_cos_missingBucketOrEndpoint(t *testing.T) {
	_, err := storage.New(storage.WithProvider("cos"), storage.WithBucket("my-bucket"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint")
}

func TestRegister_unknownProvider(t *testing.T) {
	err := storage.Register("bad", storage.WithProvider("nope"))
	require.Error(t, err)
}

func TestRegister_and_Get(t *testing.T) {
	// Register an s3 backend (with bucket; it won't make real calls)
	err := storage.Register(
		"test-s3",
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
	)
	require.NoError(t, err)

	s := storage.Get("test-s3")
	assert.NotNil(t, s)
}

func TestNew_s3_withEndpoint(t *testing.T) {
	// Custom endpoint S3-compatible (e.g., MinIO)
	s, err := storage.New(
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
		storage.WithEndpoint("http://localhost:9000"),
	)
	require.NoError(t, err)
	assert.NotNil(t, s)

	// Test that it fails gracefully on real calls without a server
	_, err = s.Download(context.Background(), "no-such-key")
	assert.Error(t, err)
}
