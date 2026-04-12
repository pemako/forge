package metric

import (
	"context"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	promexp "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	globalProvider *sdkmetric.MeterProvider
	globalMeter    otelmetric.Meter
	initialized    bool
	promSrv        *http.Server
)

// Init sets up Prometheus and/or OTLP metric exporters, registers them with
// the global OpenTelemetry MeterProvider, and wires the HTTP and gRPC
// instrumentation hooks in the middleware packages. It is safe to call Init
// without any exporters configured; in that case a no-op provider is used.
// Init must be called before any of the metric helper functions are used.
func Init(opts ...Option) error {
	o := options{serviceName: "service"}
	for _, opt := range opts {
		opt(&o)
	}

	res, _ := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(o.serviceName)),
	)

	var sdkOpts []sdkmetric.Option
	sdkOpts = append(sdkOpts, sdkmetric.WithResource(res))

	if o.prometheusAddr != "" {
		pe, err := promexp.New()
		if err != nil {
			return err
		}
		sdkOpts = append(sdkOpts, sdkmetric.WithReader(pe))

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		promSrv = &http.Server{Addr: o.prometheusAddr, Handler: mux}
		go promSrv.ListenAndServe() //nolint:errcheck
	}

	if o.otlpEndpoint != "" {
		host := otlpHost(o.otlpEndpoint)
		oe, err := otlpmetrichttp.New(context.Background(),
			otlpmetrichttp.WithEndpoint(host),
			otlpmetrichttp.WithInsecure(),
		)
		if err != nil {
			return err
		}
		sdkOpts = append(sdkOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(oe)))
	}

	mp := sdkmetric.NewMeterProvider(sdkOpts...)
	otel.SetMeterProvider(mp)
	globalProvider = mp
	globalMeter = mp.Meter(o.serviceName)
	initialized = true

	wireHTTPMetrics(globalMeter, o.serviceName)
	wireGRPCMetrics(globalMeter, o.serviceName)

	return nil
}

// Shutdown flushes any pending metric data, stops the Prometheus HTTP server
// if one was started, and releases exporter resources. Register this with
// app.WithAfterStop to ensure metrics are flushed on graceful shutdown.
func Shutdown() {
	if globalProvider != nil {
		globalProvider.Shutdown(context.Background()) //nolint:errcheck
	}
	if promSrv != nil {
		promSrv.Shutdown(context.Background()) //nolint:errcheck
	}
}

func otlpHost(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
