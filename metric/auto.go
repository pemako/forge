package metric

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	mwgrpc "github.com/pemako/mk/middleware/grpc"
	mwhttp "github.com/pemako/mk/middleware/http"
)

var (
	wireHTTPOnce sync.Once
	wireGRPCOnce sync.Once
)

func wireHTTPMetrics(meter otelmetric.Meter, svcName string) {
	wireHTTPOnce.Do(func() {
		inFlight, _ := meter.Int64UpDownCounter(svcName+"_http_requests_in_flight",
			otelmetric.WithDescription("Current HTTP requests in flight"))
		reqTotal, _ := meter.Int64Counter(svcName+"_http_requests_total",
			otelmetric.WithDescription("Total HTTP requests"))
		reqDuration, _ := meter.Float64Histogram(svcName+"_http_request_duration_seconds",
			otelmetric.WithDescription("HTTP request duration"))

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
		)
	})
}

func wireGRPCMetrics(meter otelmetric.Meter, svcName string) {
	wireGRPCOnce.Do(func() {
		inFlight, _ := meter.Int64UpDownCounter(svcName+"_grpc_requests_in_flight",
			otelmetric.WithDescription("Current gRPC requests in flight"))
		reqTotal, _ := meter.Int64Counter(svcName+"_grpc_requests_total",
			otelmetric.WithDescription("Total gRPC requests"))
		reqDuration, _ := meter.Float64Histogram(svcName+"_grpc_request_duration_seconds",
			otelmetric.WithDescription("gRPC request duration"))

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
		)
	})
}
