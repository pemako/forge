package circuitbreaker

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
)

type options struct {
	threshold     int
	timeout       time.Duration
	onStateChange func(name string, from, to State)
	cfg           core.Config // set by WithConfig; applied in New using the breaker name
	log           core.Logger
}

// Option configures a CircuitBreaker at construction time.
// Options are applied in order by New.
type Option func(*options)

// WithThreshold sets the number of consecutive failures required to open the
// circuit. Must be a positive integer. Default is 5.
// Lower values make the breaker more sensitive; higher values tolerate more
// transient errors before tripping.
func WithThreshold(n int) Option {
	return func(o *options) { o.threshold = n }
}

// WithTimeout sets how long the circuit stays open before transitioning to
// StateHalfOpen to probe whether the downstream service has recovered.
// Default is 30 s. After the timeout elapses the next Execute call acts as the
// probe; see StateHalfOpen for behaviour on success and failure.
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithOnStateChange registers a callback invoked synchronously on every state
// transition. The callback receives the breaker name and the from/to states.
// Use this to emit metrics, alerts, or structured log entries on state changes.
// The callback must not call Execute or Reset to avoid deadlock.
func WithOnStateChange(fn func(name string, from, to State)) Option {
	return func(o *options) { o.onStateChange = fn }
}

func defaultOptions() options {
	return options{threshold: 5, timeout: 30 * time.Second, log: logger.Default()}
}

// WithLogger sets the logger used for circuit state-change messages.
// Defaults to logger.Default() when not provided.
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

// WithConfig reads circuit breaker settings for the named breaker from cfg.
// New(name, WithConfig(cfg)) reads keys under "circuitbreaker.<name>.*":
//
//	circuitbreaker.<name>.threshold  — consecutive failures to open (int)
//	circuitbreaker.<name>.timeout    — open→half-open recovery delay (duration)
func WithConfig(cfg core.Config) Option {
	return func(o *options) { o.cfg = cfg }
}
