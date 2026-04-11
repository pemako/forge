package httpMiddleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Metrics records http_requests_total, http_request_duration_seconds, and
// http_requests_in_flight. Hooks are no-ops until metric.Init() wires them.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpInFlightInc()
		defer httpInFlightDec()

		start := time.Now()
		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		httpRequestsInc(c.Request.Method, c.FullPath(), status)
		httpDurationObserve(c.Request.Method, c.FullPath(), status, time.Since(start))
	}
}

var (
	httpInFlightInc     = func() {}
	httpInFlightDec     = func() {}
	httpRequestsInc     = func(method, path, status string) {}
	httpDurationObserve = func(method, path, status string, d time.Duration) {}
)

// RegisterHTTPMetricHooks is called by metric.Init() to wire real counters.
func RegisterHTTPMetricHooks(
	inFlightInc, inFlightDec func(),
	requestsInc func(method, path, status string),
	durationObserve func(method, path, status string, d time.Duration),
) {
	httpInFlightInc = inFlightInc
	httpInFlightDec = inFlightDec
	httpRequestsInc = requestsInc
	httpDurationObserve = durationObserve
}
