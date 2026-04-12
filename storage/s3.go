package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Storage is the Amazon S3 (and S3-compatible) backend for the Storage
// interface. Constructed by newS3 when the provider is "s3".
type s3Storage struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

// newS3 builds an s3Storage from the provided options using the AWS SDK v2.
// If accessKey is set, static credentials are used, bypassing the default
// credential chain. If endpoint is set it overrides the default AWS endpoint,
// enabling use of S3-compatible services such as MinIO or Cloudflare R2.
func newS3(o options) (Storage, error) {
	if o.bucket == "" {
		return nil, fmt.Errorf("storage/s3: bucket is required")
	}

	var cfgOpts []func(*awsconfig.LoadOptions) error
	if o.region != "" {
		cfgOpts = append(cfgOpts, awsconfig.WithRegion(o.region))
	}
	if o.accessKey != "" {
		cfgOpts = append(cfgOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(o.accessKey, o.secretKey, ""),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), cfgOpts...)
	if err != nil {
		return nil, fmt.Errorf("storage/s3: load config: %w", err)
	}

	var clientOpts []func(*s3.Options)
	if o.endpoint != "" {
		clientOpts = append(clientOpts, func(opt *s3.Options) {
			opt.BaseEndpoint = aws.String(o.endpoint)
		})
	}

	client := s3.NewFromConfig(cfg, clientOpts...)
	return &s3Storage{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    o.bucket,
	}, nil
}

func (s *s3Storage) Upload(ctx context.Context, key string, r io.Reader, opts ...UploadOption) (string, error) {
	uo := &uploadOpts{}
	for _, opt := range opts {
		opt(uo)
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if uo.contentType != "" {
		input.ContentType = aws.String(uo.contentType)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return "", fmt.Errorf("storage/s3: upload %q: %w", key, err)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, key), nil
}

func (s *s3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("storage/s3: download %q: %w", key, err)
	}
	return out.Body, nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage/s3: delete %q: %w", key, err)
	}
	return nil
}

func (s *s3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// 404 means not found — not an error
		return false, nil
	}
	return true, nil
}

func (s *s3Storage) PresignURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	req, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expire))
	if err != nil {
		return "", fmt.Errorf("storage/s3: presign %q: %w", key, err)
	}
	return req.URL, nil
}
