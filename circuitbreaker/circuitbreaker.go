// Package circuitbreaker implements the circuit breaker pattern for protecting
// outbound calls against cascading failures.
//
// A CircuitBreaker wraps any operation via Execute. It transitions through
// three states:
//
//   - Closed (normal): requests pass through; consecutive failures are counted.
//   - Open: requests are rejected immediately with ErrOpen after the failure
//     count reaches the threshold (default 5). The breaker stays open until
//     the timeout elapses (default 30 s).
//   - HalfOpen: after the timeout, one probe request is allowed through. On
//     success the breaker closes and resets the counter; on failure it reopens
//     and resets the timeout.
//
// Use New to create a CircuitBreaker identified by name (used in log messages
// and the OnStateChange callback). Multiple independent breakers can coexist;
// they do not share state.
//
// Example:
//
//	cb := circuitbreaker.New("payment",
//	    circuitbreaker.WithThreshold(5),
//	    circuitbreaker.WithTimeout(30*time.Second),
//	    circuitbreaker.WithOnStateChange(func(name string, from, to circuitbreaker.State) {
//	        log.Printf("circuit %s: %s → %s", name, from, to)
//	    }),
//	)
//
//	err := cb.Execute(ctx, func(ctx context.Context) error {
//	    return paymentClient.Charge(ctx, req)
//	})
//	if errors.Is(err, circuitbreaker.ErrOpen) {
//	    // Circuit is open — apply a fallback or return a cached result.
//	}
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

// ErrOpen is returned by Execute when the circuit is open and the request was
// rejected without calling the wrapped function. Callers should use
// errors.Is(err, ErrOpen) to detect this condition and apply a fallback.
var ErrOpen = errors.New("circuit breaker open")

// State represents the current circuit breaker state.
// Use State.String for a human-readable representation.
type State int

const (
	// StateClosed is the normal operating state. Requests pass through and
	// failures are counted toward the threshold.
	StateClosed State = iota
	// StateOpen rejects all requests immediately with ErrOpen. The breaker
	// transitions to StateHalfOpen after the configured timeout elapses.
	StateOpen
	// StateHalfOpen allows a single probe request to test whether the
	// downstream service has recovered. Success closes the breaker; failure
	// reopens it.
	StateHalfOpen
)

// String returns a lowercase human-readable name for the state ("closed",
// "open", or "half-open"), suitable for logging and the OnStateChange callback.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker protects an operation using the circuit breaker pattern.
// Create one with New; the zero value is not usable.
// All methods are safe for concurrent use.
type CircuitBreaker struct {
	name     string
	opts     options
	mu       sync.Mutex
	state    State
	failures int
	openAt   time.Time
	log      core.Logger
}

// New creates a CircuitBreaker identified by name using the provided options.
// name appears in log messages and the OnStateChange callback.
// Default threshold is 5 consecutive failures; default open timeout is 30 s.
// See WithThreshold, WithTimeout, and WithOnStateChange.
func New(name string, opts ...Option) *CircuitBreaker {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	// Apply config overrides using the breaker name.
	if o.cfg != nil {
		prefix := "circuitbreaker." + name + "."
		if v := o.cfg.GetInt(prefix + "threshold"); v > 0 {
			o.threshold = v
		}
		if v := o.cfg.GetDuration(prefix + "timeout"); v > 0 {
			o.timeout = v
		}
	}
	return &CircuitBreaker{
		name: name,
		opts: o,
		log:  logger.Default(),
	}
}

// Execute calls fn if the circuit is closed or half-open.
// Returns ErrOpen immediately (without calling fn) if the circuit is open.
// On success the failure counter is reset to zero; on failure the counter is
// incremented and the circuit opens when the counter reaches the threshold.
// A half-open probe failure immediately reopens the circuit regardless of the
// current counter value.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.allow(); err != nil {
		return err
	}

	err := fn(ctx)
	cb.record(ctx, err)
	return err
}

// State returns the current circuit breaker state.
// The returned value may become stale immediately after return in a concurrent
// environment; use it for monitoring and logging, not for flow control
// (Execute handles that atomically).
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// Reset forces the circuit breaker to the closed state and clears the failure
// counter. Use this in tests or after a manual remediation to immediately
// resume traffic without waiting for the timeout.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toState(StateClosed)
}

func (cb *CircuitBreaker) allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.currentState() {
	case StateOpen:
		return ErrOpen
	case StateHalfOpen:
		// Allow exactly one probe through.
		return nil
	default:
		return nil
	}
}

func (cb *CircuitBreaker) record(ctx context.Context, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.onSuccess(ctx)
	} else {
		cb.onFailure(ctx)
	}
}

func (cb *CircuitBreaker) onSuccess(ctx context.Context) {
	if cb.state == StateHalfOpen {
		cb.log.Info(ctx, "circuit breaker closed",
			core.Field{Key: "name", Value: cb.name})
		cb.toState(StateClosed)
		if cb.opts.onStateChange != nil {
			cb.opts.onStateChange(cb.name, StateHalfOpen, StateClosed)
		}
	}
	cb.failures = 0
}

func (cb *CircuitBreaker) onFailure(ctx context.Context) {
	cb.failures++
	if cb.state == StateHalfOpen || cb.failures >= cb.opts.threshold {
		cb.log.Warn(ctx, "circuit breaker opened",
			core.Field{Key: "name", Value: cb.name},
			core.Field{Key: "failures", Value: cb.failures})
		prev := cb.state
		cb.toState(StateOpen)
		if cb.opts.onStateChange != nil {
			cb.opts.onStateChange(cb.name, prev, StateOpen)
		}
	}
}

func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen && time.Since(cb.openAt) >= cb.opts.timeout {
		cb.state = StateHalfOpen
		if cb.opts.onStateChange != nil {
			cb.opts.onStateChange(cb.name, StateOpen, StateHalfOpen)
		}
	}
	return cb.state
}

func (cb *CircuitBreaker) toState(s State) {
	cb.state = s
	cb.failures = 0
	if s == StateOpen {
		cb.openAt = time.Now()
	}
}
