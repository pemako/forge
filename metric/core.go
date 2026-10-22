package metric

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/pemako/forge/core"
)

// Provider implements core.Metrics, so a package needing to record metrics can
// depend on the interface and be handed a stub in tests rather than reaching
// for this package's globals.
var _ core.Metrics = (*Provider)(nil)

// fieldAttrs converts core.Fields to OTel attributes. Values that are not
// already a supported attribute type are rendered with fmt.Sprint, matching how
// the logger treats arbitrary field values.
func fieldAttrs(labels []core.Field) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(labels))
	for _, l := range labels {
		switch v := l.Value.(type) {
		case string:
			attrs = append(attrs, attribute.String(l.Key, v))
		case bool:
			attrs = append(attrs, attribute.Bool(l.Key, v))
		case int:
			attrs = append(attrs, attribute.Int(l.Key, v))
		case int64:
			attrs = append(attrs, attribute.Int64(l.Key, v))
		case float64:
			attrs = append(attrs, attribute.Float64(l.Key, v))
		default:
			attrs = append(attrs, attribute.String(l.Key, fmt.Sprint(v)))
		}
	}
	return attrs
}

// Count adds n to the named counter. It implements core.Metrics.
// Before Init the Provider holds a no-op meter, so this is safe to call.
func (p *Provider) Count(ctx context.Context, name string, n int64, labels ...core.Field) {
	c, _ := p.Meter().Int64Counter(name)
	c.Add(ctx, n, otelmetric.WithAttributes(fieldAttrs(labels)...))
}

// Gauge sets the named gauge to value. It implements core.Metrics.
//
// It uses OTel's synchronous Float64Gauge, whose Record genuinely replaces the
// last value. An earlier version used Float64UpDownCounter.Add, which increments
// instead — so a caller following the "sets" contract got a running total.
// Use core.Metrics via a Provider for set semantics, or the package-level
// GaugeAdd/GaugeSub facades for deltas.
func (p *Provider) Gauge(ctx context.Context, name string, value float64, labels ...core.Field) {
	g, _ := p.Meter().Float64Gauge(name)
	g.Record(ctx, value, otelmetric.WithAttributes(fieldAttrs(labels)...))
}

// Histogram records value in the named histogram. It implements core.Metrics.
func (p *Provider) Histogram(ctx context.Context, name string, value float64, labels ...core.Field) {
	h, _ := p.Meter().Float64Histogram(name)
	h.Record(ctx, value, otelmetric.WithAttributes(fieldAttrs(labels)...))
}
