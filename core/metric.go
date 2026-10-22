package core

import "context"

// Metrics records application measurements. It is the metrics counterpart to
// Logger and Provider: a package can depend on this interface and be given a
// real implementation in production and a stub in tests.
//
// Previously only tracing had a core interface, so metric could not be faked at
// all — code calling the metric package's helpers emitted into a global that
// tests had no way to observe or reset.
//
// *metric.Provider implements it. Labels use Field so they read the same way as
// log fields; a nil or zero-value implementation must be safe to call.
type Metrics interface {
	// Count adds n to the named counter.
	Count(ctx context.Context, name string, n int64, labels ...Field)
	// Gauge sets the named gauge to value, replacing the previous reading.
	// Implementations must use set semantics, not accumulation.
	Gauge(ctx context.Context, name string, value float64, labels ...Field)
	// Histogram records value in the named histogram.
	Histogram(ctx context.Context, name string, value float64, labels ...Field)
}
