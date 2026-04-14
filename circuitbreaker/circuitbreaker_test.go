package circuitbreaker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pemako/forge/circuitbreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errDown = errors.New("service down")

func TestCircuitBreaker_closedOnSuccess(t *testing.T) {
	cb := circuitbreaker.New("test", circuitbreaker.WithThreshold(3))
	err := cb.Execute(context.Background(), func(_ context.Context) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())
}

func TestCircuitBreaker_opensAfterThreshold(t *testing.T) {
	cb := circuitbreaker.New("test",
		circuitbreaker.WithThreshold(3),
		circuitbreaker.WithTimeout(time.Hour),
	)
	for range 3 {
		_ = cb.Execute(context.Background(), func(_ context.Context) error { return errDown })
	}
	assert.Equal(t, circuitbreaker.StateOpen, cb.State())

	err := cb.Execute(context.Background(), func(_ context.Context) error { return nil })
	assert.ErrorIs(t, err, circuitbreaker.ErrOpen)
}

func TestCircuitBreaker_halfOpenAfterTimeout(t *testing.T) {
	cb := circuitbreaker.New("test",
		circuitbreaker.WithThreshold(1),
		circuitbreaker.WithTimeout(50*time.Millisecond),
	)
	_ = cb.Execute(context.Background(), func(_ context.Context) error { return errDown })
	assert.Equal(t, circuitbreaker.StateOpen, cb.State())

	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, circuitbreaker.StateHalfOpen, cb.State())
}

func TestCircuitBreaker_closesAfterProbeSuccess(t *testing.T) {
	cb := circuitbreaker.New("test",
		circuitbreaker.WithThreshold(1),
		circuitbreaker.WithTimeout(50*time.Millisecond),
	)
	_ = cb.Execute(context.Background(), func(_ context.Context) error { return errDown })
	time.Sleep(80 * time.Millisecond)

	err := cb.Execute(context.Background(), func(_ context.Context) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())
}

func TestCircuitBreaker_reset(t *testing.T) {
	cb := circuitbreaker.New("test", circuitbreaker.WithThreshold(1))
	_ = cb.Execute(context.Background(), func(_ context.Context) error { return errDown })
	assert.Equal(t, circuitbreaker.StateOpen, cb.State())
	cb.Reset()
	assert.Equal(t, circuitbreaker.StateClosed, cb.State())
}

func TestCircuitBreaker_onStateChange(t *testing.T) {
	var transitions []string
	cb := circuitbreaker.New("test",
		circuitbreaker.WithThreshold(2),
		circuitbreaker.WithTimeout(50*time.Millisecond),
		circuitbreaker.WithOnStateChange(func(name string, from, to circuitbreaker.State) {
			transitions = append(transitions, from.String()+"->"+to.String())
		}),
	)
	for range 2 {
		_ = cb.Execute(context.Background(), func(_ context.Context) error { return errDown })
	}
	time.Sleep(80 * time.Millisecond)
	_ = cb.State() // trigger half-open transition

	assert.Contains(t, transitions, "closed->open")
}
