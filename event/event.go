// Package event provides an in-process publish/subscribe event bus.
// Events are identified by a string topic. Handlers receive the payload as any
// and must type-assert to the expected type.
//
// A package-level default bus is available via the top-level Publish, Subscribe,
// and Unsubscribe functions. For isolated buses (e.g., in tests or multi-tenant
// scenarios where event namespaces must not bleed across), create a dedicated bus
// with New.
//
// Delivery is synchronous by default: Publish blocks until all handlers return
// and returns the first non-nil error. Use WithAsync to deliver events in
// background goroutines so that Publish returns immediately and handler errors
// are logged rather than propagated.
//
// Example:
//
//	// Register a handler on the default bus.
//	event.Subscribe("user.registered", func(ctx context.Context, payload any) error {
//	    e := payload.(UserRegisteredEvent)
//	    return mailer.SendWelcome(ctx, e.Email)
//	})
//
//	// Publish an event; blocks until all handlers finish (synchronous default).
//	err := event.Publish(ctx, "user.registered", UserRegisteredEvent{
//	    UserID: 123,
//	    Email:  "a@b.com",
//	})
//
//	// For an isolated bus with async delivery:
//	bus := event.New(event.WithAsync())
//	bus.Subscribe("order.placed", handleOrderPlaced)
package event

import (
	"context"
	"sync"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

// HandlerFunc is the signature for event handlers.
// ctx is the context passed to Publish; payload is the value passed to Publish.
// Handlers must type-assert payload to the expected concrete type.
// In synchronous mode a non-nil return value stops delivery and is returned by Publish.
// In async mode (see WithAsync) the error is logged and delivery continues.
type HandlerFunc func(ctx context.Context, payload any) error

// Bus is an in-process pub/sub event bus.
// The zero value is not usable; create one with New.
// All methods are safe for concurrent use.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]HandlerFunc
	async    bool
	log      core.Logger
}

// Option configures a Bus at construction time.
type Option func(*Bus)

// WithAsync makes the Bus deliver events in background goroutines.
// Publish returns nil immediately after spawning a goroutine per handler.
// Any handler error is logged via the configured logger instead of being
// returned to the caller. Use this option when callers must not block on
// slow or remote side-effects triggered by events.
func WithAsync() Option {
	return func(b *Bus) { b.async = true }
}

// WithLogger sets the logger used to report handler errors in async mode.
// Defaults to logger.Default() when not provided.
func WithLogger(l core.Logger) Option {
	return func(b *Bus) { b.log = l }
}

// New creates a new Bus with the given options.
// Use New instead of the default bus when you need isolated topic namespaces,
// custom async/logger settings, or deterministic teardown (e.g., in tests).
func New(opts ...Option) *Bus {
	b := &Bus{
		handlers: make(map[string][]HandlerFunc),
		log:      logger.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Subscribe registers handler for topic on this Bus.
// Multiple handlers per topic are allowed; they are invoked in registration
// order. Subscribe is safe to call concurrently with Publish.
func (b *Bus) Subscribe(topic string, handler HandlerFunc) {
	b.mu.Lock()
	b.handlers[topic] = append(b.handlers[topic], handler)
	b.mu.Unlock()
}

// Unsubscribe removes all handlers for topic from this Bus.
// Subsequent Publish calls for the same topic become no-ops.
// Unsubscribe does not affect handlers on the default bus or other Bus instances.
func (b *Bus) Unsubscribe(topic string) {
	b.mu.Lock()
	delete(b.handlers, topic)
	b.mu.Unlock()
}

// Publish delivers payload to all handlers registered for topic on this Bus.
// In synchronous mode (default) it calls handlers in registration order and
// returns the first non-nil error, stopping further delivery.
// In async mode (WithAsync) it spawns one goroutine per handler and returns nil
// immediately; handler errors are logged via the configured logger.
// If no handlers are registered for topic, Publish is a no-op and returns nil.
func (b *Bus) Publish(ctx context.Context, topic string, payload any) error {
	b.mu.RLock()
	handlers := make([]HandlerFunc, len(b.handlers[topic]))
	copy(handlers, b.handlers[topic])
	b.mu.RUnlock()

	if b.async {
		for _, h := range handlers {
			go func() {
				// Use a detached context so handler is not cancelled when
				// the caller's request context is cancelled.
				asyncCtx := context.WithoutCancel(ctx)
				if err := h(asyncCtx, payload); err != nil {
					b.log.Error(asyncCtx, "event handler error",
						core.Field{Key: "topic", Value: topic},
						core.Field{Key: "err", Value: err.Error()},
					)
				}
			}()
		}
		return nil
	}

	for _, h := range handlers {
		if err := h(ctx, payload); err != nil {
			return err
		}
	}
	return nil
}

// --- Package-level default bus ---

// _default is the shared Bus used by the package-level Subscribe, Unsubscribe,
// and Publish helpers. It uses synchronous delivery and the default logger.
var _default = New()

// Subscribe registers handler on the default bus for topic.
// See Bus.Subscribe for full semantics.
func Subscribe(topic string, handler HandlerFunc) { _default.Subscribe(topic, handler) }

// Unsubscribe removes all handlers for topic on the default bus.
// See Bus.Unsubscribe for full semantics.
func Unsubscribe(topic string) { _default.Unsubscribe(topic) }

// Publish delivers payload to all handlers registered for topic on the default bus.
// See Bus.Publish for full semantics.
func Publish(ctx context.Context, topic string, payload any) error {
	return _default.Publish(ctx, topic, payload)
}
