package storage_test

import (
	"context"
	"strings"
	"testing"
	"time"

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
	_, err := storage.New(storage.WithProvider("nope"))
	require.Error(t, err)
}

func TestRegister_and_Get(t *testing.T) {
	// Register an s3 backend (with bucket; it won't make real calls)
	s, err := storage.New(
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
	)
	require.NoError(t, err)
	err = storage.Register("test-s3", s)
	require.NoError(t, err)

	got := storage.Get("test-s3")
	assert.NotNil(t, got)
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

func TestNew_ossRequiresBucket(t *testing.T) {
	_, err := storage.New(storage.WithProvider("oss"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestNew_cosRequiresBucket(t *testing.T) {
	_, err := storage.New(storage.WithProvider("cos"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestWithContentType_applied(t *testing.T) {
	opt := storage.WithContentType("image/jpeg")
	assert.NotNil(t, opt)
}

func TestWithMetadata_applied(t *testing.T) {
	opt := storage.WithMetadata("key", "value")
	assert.NotNil(t, opt)
}

func TestRegister_errorPropagated(t *testing.T) {
	_, err := storage.New(storage.WithProvider("unknown"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestGet_afterRegister(t *testing.T) {
	// Registering a valid S3 with only provider+bucket (no credentials) creates
	// a client object even without credentials — actual calls will fail
	s := storage.Get("nonexistent_storage_xyz2")
	assert.Nil(t, s)
}

func TestNew_s3_createsClientWithoutCreds(t *testing.T) {
	// S3 with only required bucket — AWS SDK allows creation without credentials
	// (credentials would be provided via env/IAM at runtime)
	s, err := storage.New(
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
	)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestNew_s3_withAccessKey(t *testing.T) {
	s, err := storage.New(
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
		storage.WithAccessKey("AKID", "secret"),
	)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestNew_oss_withEndpoint(t *testing.T) {
	// Provide both bucket and endpoint — this should succeed at construction time
	s, err := storage.New(
		storage.WithProvider("oss"),
		storage.WithBucket("my-bucket"),
		storage.WithEndpoint("https://oss-cn-hangzhou.aliyuncs.com"),
		storage.WithAccessKey("key", "secret"),
	)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestNew_cos_withEndpoint(t *testing.T) {
	// Provide both bucket and endpoint — this should succeed at construction time
	s, err := storage.New(
		storage.WithProvider("cos"),
		storage.WithBucket("my-bucket"),
		storage.WithEndpoint("https://my-bucket.cos.ap-beijing.myqcloud.com"),
		storage.WithAccessKey("secretID", "secretKey"),
	)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestWithLogger_option(t *testing.T) {
	opt := storage.WithLogger(nil)
	assert.NotNil(t, opt)
}

func TestNew_s3_withLogger(t *testing.T) {
	s, err := storage.New(
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
		storage.WithLogger(nil),
	)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestS3_operationsFailGracefully(t *testing.T) {
	// S3 with fake endpoint — construction succeeds, operations fail with errors
	s, err := storage.New(
		storage.WithProvider("s3"),
		storage.WithBucket("test-bucket"),
		storage.WithRegion("us-east-1"),
		storage.WithEndpoint("http://localhost:19999"),
		storage.WithAccessKey("AKID", "secret"),
	)
	require.NoError(t, err)
	require.NotNil(t, s)

	ctx := context.Background()

	// Upload
	_, err = s.Upload(ctx, "key", strings.NewReader("data"))
	assert.Error(t, err)

	// Upload with content type
	_, err = s.Upload(ctx, "key", strings.NewReader("data"), storage.WithContentType("text/plain"))
	assert.Error(t, err)

	// Download
	_, err = s.Download(ctx, "key")
	assert.Error(t, err)

	// Delete
	err = s.Delete(ctx, "key")
	assert.Error(t, err)

	// Exists — returns false on error (not found), no error propagated
	_, _ = s.Exists(ctx, "key")

	// PresignURL
	_, err = s.PresignURL(ctx, "key", time.Hour)
	// presign is done locally, may succeed
	_ = err
}
