// Package storage provides a unified object storage interface for AWS S3,
// Alibaba OSS, and Tencent COS.
//
// # Instance-first usage (recommended)
//
//	s, err := storage.New(storage.WithS3(cfg))
//	// Inject s directly into services that need it.
//
// # Registry usage (convenience for small services)
//
//	storage.Register("avatars", s)
//	s := storage.Get("avatars")
package storage

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/errors"
)

// Storage is the interface implemented by all object storage backends.
// Use New to obtain a concrete implementation (S3, OSS, or COS).
type Storage interface {
	// Upload stores the content from r under key and returns the public URL.
	// Pass UploadOption values to set the Content-Type or custom metadata.
	Upload(ctx context.Context, key string, r io.Reader, opts ...UploadOption) (string, error)
	// Download retrieves the object at key as a ReadCloser.
	// The caller is responsible for closing the returned reader.
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	// Delete removes the object at key.
	// Missing keys are silently ignored (no error is returned).
	Delete(ctx context.Context, key string) error
	// Exists reports whether key exists in the bucket.
	Exists(ctx context.Context, key string) (bool, error)
	// PresignURL generates a temporary pre-signed URL for key that is valid
	// for the given expire duration, allowing unauthenticated GET access.
	PresignURL(ctx context.Context, key string, expire time.Duration) (string, error)
}

// UploadOption configures a single Upload call.
// Use WithContentType or WithMetadata to set object-level attributes.
type UploadOption func(*uploadOpts)

type uploadOpts struct {
	contentType string
	metadata    map[string]string
}

// WithContentType sets the Content-Type header stored with the uploaded object.
// When omitted, the provider may attempt content-type detection or default to
// "application/octet-stream".
func WithContentType(ct string) UploadOption {
	return func(o *uploadOpts) { o.contentType = ct }
}

// WithMetadata attaches a custom metadata key-value pair to the uploaded
// object. Call WithMetadata multiple times to set several pairs.
// Metadata is stored alongside the object and returned in HEAD/GET responses.
func WithMetadata(key, value string) UploadOption {
	return func(o *uploadOpts) {
		if o.metadata == nil {
			o.metadata = make(map[string]string)
		}
		o.metadata[key] = value
	}
}

// options holds constructor configuration.
type options struct {
	provider  string // "s3", "oss", "cos"
	region    string
	bucket    string
	accessKey string
	secretKey string
	endpoint  string // custom endpoint for S3-compatible services or OSS/COS
	log       core.Logger
}

// Option configures a Storage backend at construction time.
// Options are applied in order by New and Register.
type Option func(*options)

// WithProvider selects the storage backend. Valid values are "s3" (Amazon S3
// or any S3-compatible service), "oss" (Alibaba Cloud OSS), and "cos"
// (Tencent Cloud COS). WithProvider is required; New returns an error if it
// is not set.
func WithProvider(provider string) Option {
	return func(o *options) { o.provider = provider }
}

// WithRegion sets the cloud region (e.g., "us-east-1" for S3,
// "cn-hangzhou" for OSS). The S3 backend falls back to the AWS default
// region resolution chain when WithRegion is not provided.
func WithRegion(region string) Option {
	return func(o *options) { o.region = region }
}

// WithBucket sets the bucket name. Required for all providers; New returns
// an error when this option is absent.
func WithBucket(bucket string) Option {
	return func(o *options) { o.bucket = bucket }
}

// WithAccessKey sets the access key ID and secret access key used to
// authenticate with the storage provider. For S3 the credentials are used
// as static credentials, bypassing the default AWS credential chain.
func WithAccessKey(accessKey, secretKey string) Option {
	return func(o *options) { o.accessKey = accessKey; o.secretKey = secretKey }
}

// WithEndpoint sets a custom endpoint URL. Required for the OSS and COS
// backends (e.g., "https://oss-cn-hangzhou.aliyuncs.com" or the full COS
// bucket URL). Optional for S3-compatible services or private deployments
// (e.g., MinIO).
func WithEndpoint(endpoint string) Option {
	return func(o *options) { o.endpoint = endpoint }
}

// WithLogger sets the logger used for storage operations.
// Defaults to no logging when not provided.
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

// WithConfig reads storage settings from a core.Config instance.
// The following config keys are consulted (all under the "storage" prefix):
//
//	storage.provider   — same values as WithProvider ("s3", "oss", "cos")
//	storage.region     — cloud region
//	storage.bucket     — bucket name
//	storage.access_key — access key ID
//	storage.secret_key — secret access key
//	storage.endpoint   — custom endpoint URL
//
// Explicit option functions (WithProvider, WithBucket, etc.) applied after
// WithConfig will override the values read from config.
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetString("storage.provider"); v != "" {
			o.provider = v
		}
		if v := cfg.GetString("storage.region"); v != "" {
			o.region = v
		}
		if v := cfg.GetString("storage.bucket"); v != "" {
			o.bucket = v
		}
		if v := cfg.GetString("storage.access_key"); v != "" {
			o.accessKey = v
		}
		if v := cfg.GetString("storage.secret_key"); v != "" {
			o.secretKey = v
		}
		if v := cfg.GetString("storage.endpoint"); v != "" {
			o.endpoint = v
		}
	}
}

// New creates a Storage backend selected by WithProvider (or WithConfig).
// Returns an error if the provider is unknown, if required options such as
// bucket or endpoint are missing, or if the underlying SDK fails to
// initialise (e.g., invalid credentials or malformed endpoint URL).
func New(opts ...Option) (Storage, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	switch o.provider {
	case "s3":
		return newS3(o)
	case "oss":
		return newOSS(o)
	case "cos":
		return newCOS(o)
	default:
		return nil, fmt.Errorf("storage: unknown provider %q (use s3, oss, or cos)", o.provider)
	}
}

// --- Named registry ---

var (
	registry   = map[string]Storage{}
	registryMu sync.RWMutex
)

// ErrAlreadyRegistered is returned by Register when a storage instance with
// the given name already exists in the global registry.
var ErrAlreadyRegistered = errors.Define(409, "STORAGE_ALREADY_REGISTERED", "storage already registered")

// Register stores s in the global registry under name.
// A subsequent call to Get(name) returns the same instance.
// Register is safe for concurrent use and is typically called once at startup
// per logical storage destination (e.g., "avatars", "documents").
// Returns ErrAlreadyRegistered if name is already in use. Use Replace to overwrite.
func Register(name string, s Storage) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		return ErrAlreadyRegistered
	}
	registry[name] = s
	return nil
}

// Replace stores s under name, overwriting any existing entry.
// Storage backends hold no persistent connections that need explicit cleanup.
func Replace(name string, s Storage) error {
	registryMu.Lock()
	registry[name] = s
	registryMu.Unlock()
	return nil
}

// CloseAll removes all registered Storage instances and returns nil.
// Storage backends hold no persistent connections; this only clears the registry.
func CloseAll() error {
	registryMu.Lock()
	clear(registry)
	registryMu.Unlock()
	return nil
}

// Get returns the named Storage previously stored by Register, or nil if no
// instance has been registered under that name.
func Get(name string) Storage {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}
