// Package httpclient provides an outbound HTTP client with OTel tracing and
// exponential-backoff retry.
//
// # Instance-first usage (recommended)
//
//	c := httpclient.New(httpclient.WithBaseURL("https://api.example.com"))
//	// Inject c directly into services that need it.
//
// # Registry usage (convenience for small services)
//
//	httpclient.Register("payments", httpclient.New(httpclient.WithBaseURL(...)))
//	c := httpclient.Get("payments")
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/errors"
)

// Response holds the raw HTTP response returned by Do.
// Body contains the full, already-read response body bytes.
type Response struct {
	StatusCode int
	Body       []byte
}

// Client is a reusable HTTP client with otel tracing, exponential-backoff retry,
// and JSON marshal/unmarshal helpers. All methods are safe for concurrent use.
type Client struct {
	opts options
	http *http.Client
}

// New creates a Client with the given options.
// The underlying http.Transport is wrapped with otelhttp so every request
// carries the current trace context. Default timeout is 30 s; default retry
// count is 0 (no retry). See WithRetry, WithRetryBaseDelay, WithRetryMaxDelay
// to configure exponential-backoff retry behaviour.
func New(opts ...Option) *Client {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &Client{
		opts: o,
		http: &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

// Get performs a GET request to baseURL+path and decodes the JSON response
// body into out, which must be a non-nil pointer. Returns an error if the
// status code is >= 400.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	_, err := c.do(ctx, http.MethodGet, path, nil, out)
	return err
}

// Post performs a POST request to baseURL+path. body is JSON-marshalled and
// sent as the request body (Content-Type: application/json). The response body
// is JSON-decoded into out, which must be a non-nil pointer or nil to discard.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	_, err := c.do(ctx, http.MethodPost, path, body, out)
	return err
}

// Put performs a PUT request to baseURL+path. body is JSON-marshalled and
// sent as the request body. The response body is decoded into out.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	_, err := c.do(ctx, http.MethodPut, path, body, out)
	return err
}

// Delete performs a DELETE request to baseURL+path. If out is non-nil, the
// response body is JSON-decoded into it.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	_, err := c.do(ctx, http.MethodDelete, path, nil, out)
	return err
}

// Do performs a raw request using the given method and path. bodyBytes must be
// pre-serialised bytes or nil. The raw Response is returned on success;
// response body decoding is the caller's responsibility.
func (c *Client) Do(ctx context.Context, method, path string, bodyBytes []byte) (*Response, error) {
	return c.do(ctx, method, path, bodyBytes, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) (*Response, error) {
	var bodyBytes []byte
	if body != nil {
		switch v := body.(type) {
		case []byte:
			bodyBytes = v
		default:
			b, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("httpclient: marshal body: %w", err)
			}
			bodyBytes = b
		}
	}

	url := c.opts.baseURL + path
	var lastErr error

	for attempt := range c.opts.retryMax + 1 {
		if attempt > 0 {
			delay := backoffDelay(attempt, c.opts.retryBaseDelay, c.opts.retryMaxDelay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			c.opts.log.Debug(ctx, "httpclient retry",
				core.Field{Key: "attempt", Value: attempt},
				core.Field{Key: "url", Value: url},
			)
		}

		reqCtx, cancel := context.WithTimeout(ctx, c.opts.timeout)
		resp, err := c.attempt(reqCtx, method, url, bodyBytes, out)
		cancel()

		if err == nil {
			return resp, nil
		}
		// Only retry if warranted; otherwise return immediately.
		if resp != nil && !shouldRetry(&http.Response{StatusCode: resp.StatusCode}, nil) {
			return nil, err
		}
		if resp == nil && !shouldRetry(nil, err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, method, url string, body []byte, out any) (*Response, error) {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range c.opts.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	raw := &Response{StatusCode: resp.StatusCode, Body: respBody}

	if shouldRetry(&http.Response{StatusCode: resp.StatusCode}, nil) {
		return raw, fmt.Errorf("httpclient: status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return raw, fmt.Errorf("httpclient: status %d: %s", resp.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return raw, fmt.Errorf("httpclient: decode response: %w", err)
		}
	}
	return raw, nil
}

// --- Named registry ---

// ErrAlreadyRegistered is returned by Register when a client with the given
// name has already been registered. Use Replace to overwrite an existing entry.
var ErrAlreadyRegistered = errors.Define(409, "HTTPCLIENT_ALREADY_REGISTERED", "client already registered")

var (
	registry   = map[string]*Client{}
	registryMu sync.RWMutex
)

// Register stores c in the global registry under name.
// Returns ErrAlreadyRegistered if name is already in use. Use Replace to overwrite.
func Register(name string, c *Client) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		return ErrAlreadyRegistered
	}
	registry[name] = c
	return nil
}

// Replace stores c under name, overwriting any existing entry.
// http.Client holds no persistent connections that need explicit cleanup.
func Replace(name string, c *Client) error {
	registryMu.Lock()
	registry[name] = c
	registryMu.Unlock()
	return nil
}

// CloseAll removes all registered clients and returns nil.
// http.Client manages its own connection lifecycle; no explicit I/O release is needed.
func CloseAll() error {
	registryMu.Lock()
	clear(registry)
	registryMu.Unlock()
	return nil
}

// Get returns the named Client previously stored by Register, or nil if no
// client with that name has been registered.
func Get(name string) *Client {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// Init reads named httpclient instances from cfg and registers each one.
// names defaults to ["default"] when omitted. Config keys follow the pattern
// "httpclient.<name>.base_url", "httpclient.<name>.timeout", and
// "httpclient.<name>.retry". Each registered client is retrievable via Get.
//
// Example YAML:
//
//	httpclient:
//	  default:
//	    base_url: "https://api.example.com"
//	    timeout: 30s
//	    retry: 3
func Init(cfg core.Config, names ...string) {
	if len(names) == 0 {
		names = []string{"default"}
	}
	for _, name := range names {
		c := New(withConfigEntry(cfg, name))
		Replace(name, c) //nolint:errcheck
	}
}
