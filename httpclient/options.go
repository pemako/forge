// Package httpclient provides an outbound HTTP client with OpenTelemetry
// tracing, exponential-backoff retry, and JSON marshal/unmarshal helpers.
//
// Create a one-off client with New, or use Register/Get to manage long-lived
// named clients. For config-driven initialization, call Init with a
// core.Config and a list of names that map to "httpclient.<name>.*" keys.
//
// Typical usage:
//
//	c := httpclient.New(
//	    httpclient.WithBaseURL("https://api.example.com"),
//	    httpclient.WithTimeout(10*time.Second),
//	    httpclient.WithRetry(3),
//	)
//	var result MyResponse
//	if err := c.Get(ctx, "/v1/resource", &result); err != nil {
//	    return err
//	}
package httpclient

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
)

type options struct {
	baseURL        string
	timeout        time.Duration
	retryMax       int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	headers        map[string]string
	log            core.Logger
}

// Option configures a Client.
type Option func(*options)

// WithBaseURL sets the base URL prepended to every request path.
func WithBaseURL(url string) Option {
	return func(o *options) { o.baseURL = url }
}

// WithTimeout sets the per-request timeout applied via context (default 30s).
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithRetry sets the maximum number of retry attempts (default 0 = no retry).
func WithRetry(n int) Option {
	return func(o *options) { o.retryMax = n }
}

// WithRetryBaseDelay sets the base delay for exponential backoff (default 100ms).
func WithRetryBaseDelay(d time.Duration) Option {
	return func(o *options) { o.retryBaseDelay = d }
}

// WithRetryMaxDelay caps the backoff delay (default 2s).
func WithRetryMaxDelay(d time.Duration) Option {
	return func(o *options) { o.retryMaxDelay = d }
}

// WithHeader adds a default header sent on every request.
func WithHeader(key, value string) Option {
	return func(o *options) {
		if o.headers == nil {
			o.headers = make(map[string]string)
		}
		o.headers[key] = value
	}
}

// WithLogger sets the logger used for retry debug logging.
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

// withConfigEntry reads httpclient.<name>.* keys from cfg and applies them as
// options. Keys recognised: base_url (string), timeout (duration), retry (int).
// Unknown keys are silently ignored.
func withConfigEntry(cfg core.Config, name string) Option {
	return func(o *options) {
		prefix := "httpclient." + name + "."
		if v := cfg.GetString(prefix + "base_url"); v != "" {
			o.baseURL = v
		}
		if v := cfg.GetDuration(prefix + "timeout"); v > 0 {
			o.timeout = v
		}
		if v := cfg.GetInt(prefix + "retry"); v > 0 {
			o.retryMax = v
		}
	}
}

func defaultOptions() options {
	return options{
		timeout:        30 * time.Second,
		retryBaseDelay: 100 * time.Millisecond,
		retryMaxDelay:  2 * time.Second,
		log:            logger.Default(),
	}
}
