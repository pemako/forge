// Package httpclient/retry.go
// Retry helpers for httpclient. These are intentionally unexported; external
// consumers configure retry behaviour through WithRetry, WithRetryBaseDelay,
// and WithRetryMaxDelay options.
package httpclient

import (
	"math/rand/v2"
	"net/http"
	"time"
)

// shouldRetry reports whether the response warrants a retry.
// Retries on network errors (resp==nil), 429, and 5xx.
// Never retries 4xx (except 429).
func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
}

// backoffDelay returns the wait before attempt n (1-based) using exponential
// backoff with ±25% jitter, capped at maxDelay.
func backoffDelay(attempt int, base, maxDelay time.Duration) time.Duration {
	shift := attempt - 1
	if shift > 10 {
		shift = 10
	}
	d := base * (1 << shift)
	if d > maxDelay {
		d = maxDelay
	}
	// jitter: uniform in [-d/4, +d/4]
	jitter := time.Duration(rand.Int64N(int64(d/2+1))) - d/4
	result := d + jitter
	if result < 0 {
		return 0
	}
	return result
}

// ExportedBackoffDelay exposes backoffDelay for white-box testing.
// External packages should not rely on this function; it may be removed in a
// future version.
func ExportedBackoffDelay(attempt int, base, maxDelay time.Duration) time.Duration {
	return backoffDelay(attempt, base, maxDelay)
}
