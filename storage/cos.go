package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// cosStorage is the Tencent Cloud COS backend for the Storage interface.
// Constructed by newCOS when the provider is "cos".
type cosStorage struct {
	client    *cos.Client
	bucket    string
	secretID  string
	secretKey string
}

// newCOS builds a cosStorage. Both bucket and endpoint are required; the
// endpoint must be the full bucket URL in the form
// "https://<bucket>.cos.<region>.myqcloud.com".
func newCOS(o options) (Storage, error) {
	if o.bucket == "" || o.endpoint == "" {
		return nil, fmt.Errorf("storage/cos: bucket and endpoint are required")
	}
	u, err := url.Parse(o.endpoint)
	if err != nil {
		return nil, fmt.Errorf("storage/cos: parse endpoint: %w", err)
	}
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  o.accessKey,
			SecretKey: o.secretKey,
		},
	})
	return &cosStorage{
		client:    client,
		bucket:    o.bucket,
		secretID:  o.accessKey,
		secretKey: o.secretKey,
	}, nil
}

func (s *cosStorage) Upload(ctx context.Context, key string, r io.Reader, opts ...UploadOption) (string, error) {
	uo := &uploadOpts{}
	for _, opt := range opts {
		opt(uo)
	}
	putOpt := &cos.ObjectPutOptions{}
	if uo.contentType != "" {
		putOpt.ObjectPutHeaderOptions = &cos.ObjectPutHeaderOptions{ContentType: uo.contentType}
	}
	if _, err := s.client.Object.Put(ctx, key, r, putOpt); err != nil {
		return "", fmt.Errorf("storage/cos: upload %q: %w", key, err)
	}
	return fmt.Sprintf("%s/%s", s.client.BaseURL.BucketURL.String(), key), nil
}

func (s *cosStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("storage/cos: download %q: %w", key, err)
	}
	return resp.Body, nil
}

func (s *cosStorage) Delete(ctx context.Context, key string) error {
	if _, err := s.client.Object.Delete(ctx, key); err != nil {
		return fmt.Errorf("storage/cos: delete %q: %w", key, err)
	}
	return nil
}

func (s *cosStorage) Exists(ctx context.Context, key string) (bool, error) {
	ok, err := s.client.Object.IsExist(ctx, key)
	if err != nil {
		return false, fmt.Errorf("storage/cos: exists %q: %w", key, err)
	}
	return ok, nil
}

func (s *cosStorage) PresignURL(_ context.Context, key string, expire time.Duration) (string, error) {
	presignedURL, err := s.client.Object.GetPresignedURL(
		context.Background(), http.MethodGet, key,
		s.secretID,
		s.secretKey,
		expire, nil,
	)
	if err != nil {
		return "", fmt.Errorf("storage/cos: presign %q: %w", key, err)
	}
	return presignedURL.String(), nil
}
