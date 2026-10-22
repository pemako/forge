package httpMiddleware

import (
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Metrics records http_requests_total, http_request_duration_seconds,
// http_requests_in_flight, and http_errors_total. The hooks are no-ops until
// metric.Init wires them.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Load the hook set once per request so a concurrent rewire cannot
		// observe a half-updated set.
		h := httpHooks()
		h.inFlightInc()
		defer h.inFlightDec()

		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		h.requestsInc(c.Request.Method, c.FullPath(), status)
		h.durationObserve(c.Request.Method, c.FullPath(), status, time.Since(start))
	}
}

// httpMetricHooks is the full set of metric callbacks used by the HTTP
// middleware. It is swapped as one immutable value so the request path never
// reads a partially-wired set.
//
// It replaces four individually-assigned package vars, which the race detector
// flagged: metric.Init wrote them while in-flight requests were reading them.
type httpMetricHooks struct {
	inFlightInc     func()
	inFlightDec     func()
	requestsInc     func(method, path, status string)
	durationObserve func(method, path, status string, d time.Duration)
	errorsInc       func(reason, code string)
}

var noopHTTPHooks = &httpMetricHooks{
	inFlightInc:     func() {},
	inFlightDec:     func() {},
	requestsInc:     func(string, string, string) {},
	durationObserve: func(string, string, string, time.Duration) {},
	errorsInc:       func(string, string) {},
}

var httpHooksHolder atomic.Pointer[httpMetricHooks]

func httpHooks() *httpMetricHooks {
	if h := httpHooksHolder.Load(); h != nil {
		return h
	}
	return noopHTTPHooks
}

// RegisterHTTPMetricHooks is called by metric.Init to wire real instruments.
// Any nil callback is replaced with a no-op, so partially-wired sets are safe.
//
// Calling it more than once replaces the previous set; the most recent call
// wins. It is safe to call while requests are being served.
func RegisterHTTPMetricHooks(
	inFlightInc, inFlightDec func(),
	requestsInc func(method, path, status string),
	durationObserve func(method, path, status string, d time.Duration),
	errorsInc func(reason, code string),
) {
	h := &httpMetricHooks{
		inFlightInc:     inFlightInc,
		inFlightDec:     inFlightDec,
		requestsInc:     requestsInc,
		durationObserve: durationObserve,
		errorsInc:       errorsInc,
	}
	if h.inFlightInc == nil {
		h.inFlightInc = noopHTTPHooks.inFlightInc
	}
	if h.inFlightDec == nil {
		h.inFlightDec = noopHTTPHooks.inFlightDec
	}
	if h.requestsInc == nil {
		h.requestsInc = noopHTTPHooks.requestsInc
	}
	if h.durationObserve == nil {
		h.durationObserve = noopHTTPHooks.durationObserve
	}
	if h.errorsInc == nil {
		h.errorsInc = noopHTTPHooks.errorsInc
	}
	httpHooksHolder.Store(h)
}

// ResetHTTPMetricHooks restores the no-op hook set. It exists so tests can
// unwire instruments that metric.Init installed; without it the first Init in a
// test binary leaked its instruments into every later test.
func ResetHTTPMetricHooks() { httpHooksHolder.Store(noopHTTPHooks) }
