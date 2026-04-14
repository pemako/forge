// Package grpcclient provides a gRPC client factory with OpenTelemetry tracing,
// structured request logging, exponential-backoff retry, and optional TLS/mTLS.
//
// Use Dial to create a single *grpc.ClientConn, or Register/Get to manage a
// global registry of named connections. For config-driven initialization, call
// Init with a compatible config object and a list of service names that map to
// "grpcclient.<name>.*" keys.
//
// Typical usage:
//
//	conn, err := grpcclient.Dial("user-service:9090",
//	    grpcclient.WithInsecure(),
//	    grpcclient.WithTimeout(5*time.Second),
//	    grpcclient.WithRetry(3),
//	)
//	if err != nil {
//	    return err
//	}
//	defer conn.Close()
//	client := userpb.NewUserServiceClient(conn)
package grpcclient

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
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

// WithTLS configures one-way TLS using the given CA certificate file path.
// The server's certificate must be signed by caCert.
func WithTLS(caCert string) Option { return func(o *options) { o.tlsCACert = caCert } }

// WithMTLS configures mutual TLS. cert and key are paths to the client
// certificate and private key; caCert is the CA that signed the server cert.
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
// Retryable codes are Unavailable, DeadlineExceeded, and ResourceExhausted.
func WithRetry(n int) Option { return func(o *options) { o.retryMax = n } }

// WithRetryBaseDelay sets the exponential backoff base delay (default 100ms).
// Use together with WithRetry and WithRetryMaxDelay to tune the backoff schedule.
func WithRetryBaseDelay(d time.Duration) Option { return func(o *options) { o.retryBaseDelay = d } }

// WithRetryMaxDelay caps the maximum backoff delay between retry attempts (default 2s).
// Use together with WithRetry and WithRetryBaseDelay to tune the backoff schedule.
func WithRetryMaxDelay(d time.Duration) Option { return func(o *options) { o.retryMaxDelay = d } }

// WithLogger sets the logger used for per-RPC request logging (method, latency, error).
func WithLogger(l core.Logger) Option { return func(o *options) { o.log = l } }

func defaultOptions() options {
	return options{
		timeout:        30 * time.Second,
		retryBaseDelay: 100 * time.Millisecond,
		retryMaxDelay:  2 * time.Second,
		log:            logger.Default(),
	}
}
