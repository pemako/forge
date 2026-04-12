package event_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pemako/mk/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBus_subscribe_publish(t *testing.T) {
	b := event.New()
	var got string
	b.Subscribe("greet", func(_ context.Context, payload any) error {
		got = payload.(string)
		return nil
	})
	require.NoError(t, b.Publish(context.Background(), "greet", "hello"))
	assert.Equal(t, "hello", got)
}

func TestBus_multipleHandlers(t *testing.T) {
	b := event.New()
	var calls atomic.Int32
	for range 3 {
		b.Subscribe("tick", func(_ context.Context, _ any) error {
			calls.Add(1)
			return nil
		})
	}
	require.NoError(t, b.Publish(context.Background(), "tick", nil))
	assert.Equal(t, int32(3), calls.Load())
}

func TestBus_handlerError_stopsChain(t *testing.T) {
	b := event.New()
	var second bool
	b.Subscribe("fail", func(_ context.Context, _ any) error { return errors.New("boom") })
	b.Subscribe("fail", func(_ context.Context, _ any) error { second = true; return nil })
	err := b.Publish(context.Background(), "fail", nil)
	assert.Error(t, err)
	assert.False(t, second, "second handler must not run after first returns error")
}

func TestBus_noHandlers_noError(t *testing.T) {
	b := event.New()
	assert.NoError(t, b.Publish(context.Background(), "unknown", nil))
}

func TestBus_unsubscribe(t *testing.T) {
	b := event.New()
	var called bool
	b.Subscribe("ev", func(_ context.Context, _ any) error { called = true; return nil })
	b.Unsubscribe("ev")
	_ = b.Publish(context.Background(), "ev", nil)
	assert.False(t, called)
}

func TestBus_async(t *testing.T) {
	b := event.New(event.WithAsync())
	var count atomic.Int32
	for range 3 {
		b.Subscribe("async", func(_ context.Context, _ any) error {
			count.Add(1)
			return nil
		})
	}
	require.NoError(t, b.Publish(context.Background(), "async", nil))
	// give goroutines time to run
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(3), count.Load())
}

func TestDefaultBus(t *testing.T) {
	var got int
	event.Subscribe("default.test", func(_ context.Context, payload any) error {
		got = payload.(int)
		return nil
	})
	defer event.Unsubscribe("default.test")
	require.NoError(t, event.Publish(context.Background(), "default.test", 42))
	assert.Equal(t, 42, got)
}
