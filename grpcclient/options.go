// grpcclient/options.go
package grpcclient

import (
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

type options struct {
	insecure       bool
	tlsCACert      string
	tlsCert        string
	tlsKey         string
	timeout        time.Duration
	retryMax       int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	log            core.Logger
}

// Option configures a gRPC client connection.
type Option func(*options)

// WithInsecure disables TLS (plaintext). Use in development or trusted internal networks.
func WithInsecure() Option { return func(o *options) { o.insecure = true } }

// WithTLS configures one-way TLS using the given CA certificate file.
func WithTLS(caCert string) Option { return func(o *options) { o.tlsCACert = caCert } }

// WithMTLS configures mutual TLS.
func WithMTLS(cert, key, caCert string) Option {
	return func(o *options) {
		o.tlsCert = cert
		o.tlsKey = key
		o.tlsCACert = caCert
	}
}

// WithTimeout sets a per-call deadline injected into the outgoing context (default 30s).
func WithTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// WithRetry sets the maximum number of retry attempts for retryable status codes (default 0).
func WithRetry(n int) Option { return func(o *options) { o.retryMax = n } }

// WithRetryBaseDelay sets the exponential backoff base delay (default 100ms).
func WithRetryBaseDelay(d time.Duration) Option { return func(o *options) { o.retryBaseDelay = d } }

// WithLogger sets the logger for RPC request logging.
func WithLogger(l core.Logger) Option { return func(o *options) { o.log = l } }

func defaultOptions() options {
	return options{
		timeout:        30 * time.Second,
		retryBaseDelay: 100 * time.Millisecond,
		retryMaxDelay:  2 * time.Second,
		log:            logger.Default(),
	}
}
