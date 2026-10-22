package metric

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	mwgrpc "github.com/pemako/forge/middleware/grpc"
	mwhttp "github.com/pemako/forge/middleware/http"
)

// wireHTTPMetrics installs real HTTP instruments into the middleware package.
//
// This used to be guarded by a package-level sync.Once, which meant only the
// first Provider in a process ever wired anything: a second Provider — another
// app instance, or the next test in the same binary — was silently ignored and
// kept reporting through the first one's instruments, under the first one's
// service name. The middleware hook set is now swapped atomically, so rewiring
// is safe and the most recent Init wins.
func wireHTTPMetrics(meter otelmetric.Meter, svcName string) {
	inFlight, _ := meter.Int64UpDownCounter(svcName+"_http_requests_in_flight",
		otelmetric.WithDescription("Current HTTP requests in flight"))
	reqTotal, _ := meter.Int64Counter(svcName+"_http_requests_total",
		otelmetric.WithDescription("Total HTTP requests"))
	reqDuration, _ := meter.Float64Histogram(svcName+"_http_request_duration_seconds",
		otelmetric.WithDescription("HTTP request duration"))
	errTotal, _ := meter.Int64Counter(svcName+"_http_errors_total",
		otelmetric.WithDescription("Total HTTP responses carrying an error, by reason"))

	mwhttp.RegisterHTTPMetricHooks(
		func() { inFlight.Add(context.Background(), 1) },
		func() { inFlight.Add(context.Background(), -1) },
		func(method, path, status string) {
			reqTotal.Add(context.Background(), 1,
				otelmetric.WithAttributes(
					attribute.String("method", method),
					attribute.String("path", path),
					attribute.String("status", status),
				))
		},
		func(method, path, status string, d time.Duration) {
			reqDuration.Record(context.Background(), d.Seconds(),
				otelmetric.WithAttributes(
					attribute.String("method", method),
					attribute.String("path", path),
					attribute.String("status", status),
				))
		},
		func(reason, code string) {
			errTotal.Add(context.Background(), 1,
				otelmetric.WithAttributes(
					attribute.String("reason", reason),
					attribute.String("code", code),
				))
		},
	)
}

// wireGRPCMetrics installs real gRPC instruments into the middleware package.
// Like wireHTTPMetrics it is no longer one-shot; see that function's comment.
func wireGRPCMetrics(meter otelmetric.Meter, svcName string) {
	inFlight, _ := meter.Int64UpDownCounter(svcName+"_grpc_requests_in_flight",
		otelmetric.WithDescription("Current gRPC requests in flight"))
	reqTotal, _ := meter.Int64Counter(svcName+"_grpc_requests_total",
		otelmetric.WithDescription("Total gRPC requests"))
	reqDuration, _ := meter.Float64Histogram(svcName+"_grpc_request_duration_seconds",
		otelmetric.WithDescription("gRPC request duration"))
	errTotal, _ := meter.Int64Counter(svcName+"_grpc_errors_total",
		otelmetric.WithDescription("Total failed gRPC requests, by status code"))

	mwgrpc.RegisterGRPCMetricHooks(
		func(method string) {
			inFlight.Add(context.Background(), 1,
				otelmetric.WithAttributes(attribute.String("method", method)))
		},
		func(method string) {
			inFlight.Add(context.Background(), -1,
				otelmetric.WithAttributes(attribute.String("method", method)))
		},
		func(method, status string) {
			reqTotal.Add(context.Background(), 1,
				otelmetric.WithAttributes(
					attribute.String("method", method),
					attribute.String("status", status),
				))
		},
		func(method, status string, d time.Duration) {
			reqDuration.Record(context.Background(), d.Seconds(),
				otelmetric.WithAttributes(
					attribute.String("method", method),
					attribute.String("status", status),
				))
		},
		func(method, status string) {
			errTotal.Add(context.Background(), 1,
				otelmetric.WithAttributes(
					attribute.String("method", method),
					attribute.String("status", status),
				))
		},
	)
}
