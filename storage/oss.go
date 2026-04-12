package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// ossStorage is the Alibaba Cloud OSS backend for the Storage interface.
// Constructed by newOSS when the provider is "oss".
type ossStorage struct {
	client *oss.Client
	bucket *oss.Bucket
	name   string
}

// newOSS builds an ossStorage. Both bucket and endpoint are required; the
// endpoint must be a full OSS service URL (e.g.,
// "https://oss-cn-hangzhou.aliyuncs.com").
func newOSS(o options) (Storage, error) {
	if o.bucket == "" || o.endpoint == "" {
		return nil, fmt.Errorf("storage/oss: bucket and endpoint are required")
	}
	client, err := oss.New(o.endpoint, o.accessKey, o.secretKey)
	if err != nil {
		return nil, fmt.Errorf("storage/oss: create client: %w", err)
	}
	bucket, err := client.Bucket(o.bucket)
	if err != nil {
		return nil, fmt.Errorf("storage/oss: open bucket: %w", err)
	}
	return &ossStorage{client: client, bucket: bucket, name: o.bucket}, nil
}

func (s *ossStorage) Upload(_ context.Context, key string, r io.Reader, opts ...UploadOption) (string, error) {
	uo := &uploadOpts{}
	for _, opt := range opts {
		opt(uo)
	}
	var ossOpts []oss.Option
	if uo.contentType != "" {
		ossOpts = append(ossOpts, oss.ContentType(uo.contentType))
	}
	if err := s.bucket.PutObject(key, r, ossOpts...); err != nil {
		return "", fmt.Errorf("storage/oss: upload %q: %w", key, err)
	}
	return fmt.Sprintf("https://%s/%s", s.name, key), nil
}

func (s *ossStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.bucket.GetObject(key)
	if err != nil {
		return nil, fmt.Errorf("storage/oss: download %q: %w", key, err)
	}
	return obj, nil
}

func (s *ossStorage) Delete(_ context.Context, key string) error {
	if err := s.bucket.DeleteObject(key); err != nil {
		return fmt.Errorf("storage/oss: delete %q: %w", key, err)
	}
	return nil
}

func (s *ossStorage) Exists(_ context.Context, key string) (bool, error) {
	ok, err := s.bucket.IsObjectExist(key)
	if err != nil {
		return false, fmt.Errorf("storage/oss: exists %q: %w", key, err)
	}
	return ok, nil
}

func (s *ossStorage) PresignURL(_ context.Context, key string, expire time.Duration) (string, error) {
	url, err := s.bucket.SignURL(key, oss.HTTPGet, int64(expire.Seconds()))
	if err != nil {
		return "", fmt.Errorf("storage/oss: presign %q: %w", key, err)
	}
	return url, nil
}
