package metric

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	promexp "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Provider wraps OpenTelemetry metric exporters and the optional Prometheus
// HTTP server. It implements the app.Lifecycle interface (Init + Shutdown).
// Create one with New; pass Init and Shutdown to app lifecycle hooks.
type Provider struct {
	opts    options
	mp      *sdkmetric.MeterProvider
	meter   otelmetric.Meter
	promSrv *http.Server
}

// New creates a Provider with the given options.
// No exporters are started until Init is called.
func New(opts ...Option) (*Provider, error) {
	o := options{serviceName: "service"}
	for _, opt := range opts {
		opt(&o)
	}
	return &Provider{opts: o}, nil
}

// Meter returns the otelmetric.Meter for this Provider.
// Returns a no-op meter if Init has not been called.
func (p *Provider) Meter() otelmetric.Meter {
	if p.mp == nil {
		return noopMeter()
	}
	return p.meter
}

// Init sets up exporters, registers the MeterProvider with the global OTel
// registry, and wires middleware instrumentation hooks. It implements the
// app.Lifecycle Init method.
// Calling Init more than once on the same Provider is a no-op.
func (p *Provider) Init(ctx context.Context) error {
	if p.mp != nil {
		// Already initialized.
		return nil
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(p.opts.serviceName)),
	)

	var sdkOpts []sdkmetric.Option
	sdkOpts = append(sdkOpts, sdkmetric.WithResource(res))

	if p.opts.prometheusAddr != "" {
		pe, err := promexp.New()
		if err != nil {
			return err
		}
		sdkOpts = append(sdkOpts, sdkmetric.WithReader(pe))

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		p.promSrv = &http.Server{Addr: p.opts.prometheusAddr, Handler: mux}
		go p.promSrv.ListenAndServe() //nolint:errcheck
	}

	if p.opts.otlpEndpoint != "" {
		host := otlpHost(p.opts.otlpEndpoint)
		oe, err := otlpmetrichttp.New(ctx,
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
	p.mp = mp
	p.meter = mp.Meter(p.opts.serviceName)

	wireHTTPMetrics(p.meter, p.opts.serviceName)
	wireGRPCMetrics(p.meter, p.opts.serviceName)

	return nil
}

// Shutdown flushes pending metric data, stops the Prometheus HTTP server if
// one was started, and releases exporter resources. It implements the
// app.Lifecycle Shutdown method.
func (p *Provider) Shutdown(ctx context.Context) error {
	var first error
	if p.mp != nil {
		if err := p.mp.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	if p.promSrv != nil {
		if err := p.promSrv.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// --- Package-level globals (wraps a Provider for backward compatibility) ---

var (
	globalProvider *Provider
	globalMeter    otelmetric.Meter
	initialized    bool
	globalMu       sync.Mutex
)

// Init sets up Prometheus and/or OTLP metric exporters, registers them with
// the global OpenTelemetry MeterProvider, and wires the HTTP and gRPC
// instrumentation hooks in the middleware packages. It is safe to call Init
// without any exporters configured; in that case a no-op provider is used.
// Init must be called before any of the metric helper functions are used.
func Init(opts ...Option) error {
	p, err := New(opts...)
	if err != nil {
		return err
	}
	if err := p.Init(context.Background()); err != nil {
		return err
	}
	globalMu.Lock()
	globalProvider = p
	globalMeter = p.meter
	initialized = true
	globalMu.Unlock()
	return nil
}

// Shutdown flushes any pending metric data, stops the Prometheus HTTP server
// if one was started, and releases exporter resources. Register this with
// app.WithAfterStop to ensure metrics are flushed on graceful shutdown.
func Shutdown(ctx context.Context) error {
	globalMu.Lock()
	p := globalProvider
	globalMu.Unlock()
	if p == nil {
		return nil
	}
	return p.Shutdown(ctx)
}

func otlpHost(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
