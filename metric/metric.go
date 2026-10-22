package metric

import (
	"context"
	stderrors "errors"
	"net"
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

	ferrors "github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
)

// ErrPrometheusListen is returned by Init when the Prometheus /metrics listener
// cannot bind its configured address.
var ErrPrometheusListen = ferrors.Define(500, "METRIC_PROMETHEUS_LISTEN", "prometheus listener failed")

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

// ErrNoExporters is returned by Init when neither a Prometheus address nor an
// OTLP endpoint is configured. A MeterProvider with no reader silently discards
// every measurement, so this is reported rather than left to be discovered from
// an empty dashboard. Pass WithAllowNoExporters to opt out.
var ErrNoExporters = ferrors.Define(500, "METRIC_NO_EXPORTERS",
	"no metric exporter configured: set metric.prometheus_addr or metric.otlp_endpoint, or pass WithAllowNoExporters")

// Init sets up exporters, registers the MeterProvider with the global OTel
// registry, and wires middleware instrumentation hooks. It implements the
// app.Lifecycle Init method.
// Calling Init more than once on the same Provider is a no-op.
//
// Init returns ErrNoExporters when no exporter is configured, and fails if the
// Prometheus listener cannot bind — the address is bound synchronously so a port
// clash surfaces here instead of leaving /metrics quietly unserved.
func (p *Provider) Init(ctx context.Context) error {
	if p.mp != nil {
		// Already initialized.
		return nil
	}

	if p.opts.prometheusAddr == "" && p.opts.otlpEndpoint == "" &&
		len(p.opts.readers) == 0 && !p.opts.allowNoExporters {
		return ErrNoExporters
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(p.opts.serviceName)),
	)

	var sdkOpts []sdkmetric.Option
	sdkOpts = append(sdkOpts, sdkmetric.WithResource(res))

	for _, r := range p.opts.readers {
		sdkOpts = append(sdkOpts, sdkmetric.WithReader(r))
	}

	if p.opts.prometheusAddr != "" {
		pe, err := promexp.New()
		if err != nil {
			return err
		}
		sdkOpts = append(sdkOpts, sdkmetric.WithReader(pe))

		// Bind synchronously so an address clash is reported from Init. This
		// used to be `go p.promSrv.ListenAndServe()` with the error discarded,
		// which meant a taken port left /metrics permanently unserved while the
		// service reported itself healthy.
		lis, err := net.Listen("tcp", p.opts.prometheusAddr)
		if err != nil {
			return ferrors.Wrap(ErrPrometheusListen, "metric: listen %s: %w", p.opts.prometheusAddr, err)
		}

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		p.promSrv = &http.Server{Handler: mux}
		go func() {
			if err := p.promSrv.Serve(lis); err != nil && !stderrors.Is(err, http.ErrServerClosed) {
				logger.Error(context.Background(), "metric: prometheus server stopped",
					logger.F("error", err))
			}
		}()
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
