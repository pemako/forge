// Package stream provides distributed pub/sub and work-queue messaging
// backed by Redis Streams (XADD / XREADGROUP). It requires Redis 5.0+.
//
// Two delivery modes:
//   - Work queue (WithGroup): only ONE instance processes each message.
//   - Broadcast (WithBroadcast): EVERY instance receives each message.
//
// The Redis client is provided by the caller — typically via
// cache.RedisClient("name") to share an existing connection:
//
//	stream.Init(cache.RedisClient("redis"))
//	stream.Subscribe("order.created",
//	    stream.WithGroup("order-service"),
//	    stream.WithHandler(handler),
//	)
//	app.New(app.WithServer(stream.Worker()))
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/pemako/mk/core"
	"github.com/redis/go-redis/v9"
)

// HandlerFunc processes a single stream message.
// Return nil to acknowledge (XACK) the message.
// Return an error to trigger a retry (up to WithRetries attempts).
type HandlerFunc func(ctx context.Context, msg Message) error

// Message is a single entry read from a Redis Stream.
type Message struct {
	// ID is the Redis stream message ID (e.g., "1695000000000-0").
	ID string
	// Topic is the stream name the message was read from.
	Topic string
	// Payload is the raw JSON-encoded message body.
	Payload []byte
}

// Decode unmarshals the message payload into dest (must be a pointer).
func (m Message) Decode(dest any) error {
	return json.Unmarshal(m.Payload, dest)
}

// Bus is a self-contained Redis Streams pub/sub instance.
// Use New to create an isolated Bus (e.g., in tests or multi-tenant scenarios).
// The package-level Init/Publish/Subscribe/Worker functions operate on a
// shared default Bus.
type Bus struct {
	rdb  *redis.Client
	opts initOptions
	mu   sync.RWMutex
	subs []subscription
}

// New creates a new Bus connected to the given Redis client.
func New(client *redis.Client, options ...InitOption) *Bus {
	o := defaultInitOptions()
	for _, opt := range options {
		opt(&o)
	}
	return &Bus{rdb: client, opts: o}
}

// Publish writes payload to the Redis Stream identified by topic.
// payload is JSON-marshalled before writing. The stream is trimmed
// approximately to the MaxLen configured in New (default 10 000).
func (b *Bus) Publish(ctx context.Context, topic string, payload any) error {
	if b.rdb == nil {
		return fmt.Errorf("stream: Redis client is nil")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("stream: marshal payload: %w", err)
	}
	return b.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		MaxLen: b.opts.maxLen,
		Approx: true,
		Values: map[string]any{"data": string(data)},
	}).Err()
}

// Subscribe registers a handler for the given topic on this Bus.
// Must be called before Worker().Start — subscriptions registered after Start
// are not picked up by the running Worker.
//
// Use WithGroup for work-queue delivery (one instance processes each message).
// Use WithBroadcast for fan-out delivery (every instance receives each message).
func (b *Bus) Subscribe(topic string, opts ...SubOption) {
	s := defaultSubOptions(topic)
	for _, opt := range opts {
		opt(&s)
	}

	if s.broadcast {
		// Unique group per instance so every instance gets its own copy.
		h, _ := os.Hostname()
		s.group = fmt.Sprintf("%s:broadcast:%s", topic, h)
		s.consumer = h
	}

	if s.group == "" {
		// Default group = topic name (reasonable for single-service work queues).
		s.group = topic
	}

	b.mu.Lock()
	b.subs = append(b.subs, s)
	b.mu.Unlock()
}

// Worker returns a core.Server that reads from all registered subscriptions
// on this Bus. Add it to app.WithServer to integrate with the application
// lifecycle. Worker.Start blocks until Worker.Stop is called.
func (b *Bus) Worker() *streamWorker {
	return &streamWorker{
		bus:  b,
		done: make(chan struct{}),
		log:  b.opts.log,
	}
}

// --- package-level default Bus ---

var _default *Bus

// Init sets the Redis client used for all stream operations.
// Must be called once before Publish, Subscribe, or Worker.
//
// The recommended pattern is to share the connection via the cache package:
//
//	stream.Init(cache.RedisClient("redis"))
//
// where "redis" is a named Redis backend registered in cache.Init.
// This avoids duplicating the Redis address in the configuration.
func Init(client *redis.Client, options ...InitOption) {
	_default = New(client, options...)
}

// Publish writes payload to the Redis Stream identified by topic on the
// default Bus. payload is JSON-marshalled before writing.
func Publish(ctx context.Context, topic string, payload any) error {
	if _default == nil {
		return fmt.Errorf("stream: not initialized — call stream.Init first")
	}
	return _default.Publish(ctx, topic, payload)
}

// Subscribe registers a handler for the given topic on the default Bus.
func Subscribe(topic string, opts ...SubOption) {
	if _default == nil {
		return
	}
	_default.Subscribe(topic, opts...)
}

// Worker returns a core.Server that reads from all registered subscriptions
// on the default Bus.
func Worker(l ...core.Logger) *streamWorker {
	if _default == nil {
		// Return a worker that does nothing until Init is called.
		b := &Bus{opts: defaultInitOptions()}
		if len(l) > 0 && l[0] != nil {
			b.opts.log = l[0]
		}
		return b.Worker()
	}
	w := _default.Worker()
	if len(l) > 0 && l[0] != nil {
		w.log = l[0]
	}
	return w
}

// ResetSubs clears all registered subscriptions on the default Bus.
// Intended for use in tests only to isolate test cases that share global state.
func ResetSubs() {
	if _default != nil {
		_default.mu.Lock()
		_default.subs = _default.subs[:0]
		_default.mu.Unlock()
	}
}
