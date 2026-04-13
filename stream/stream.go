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
//	bus, err := stream.New(cache.RedisClient("redis"))
//	if err != nil { ... }
//	bus.Subscribe("order.created",
//	    stream.WithGroup("order-service"),
//	    stream.WithHandler(handler),
//	)
//	app.New(app.WithServer(bus.Worker()))
package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

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
// Use New to create a Bus.
type Bus struct {
	rdb  *redis.Client
	opts initOptions
	mu   sync.RWMutex
	subs []subscription
}

// New creates a new Bus connected to the given Redis client.
// Returns an error if client is nil.
func New(client *redis.Client, options ...InitOption) (*Bus, error) {
	if client == nil {
		return nil, fmt.Errorf("stream: Redis client must not be nil")
	}
	o := defaultInitOptions()
	for _, opt := range options {
		opt(&o)
	}
	return &Bus{rdb: client, opts: o}, nil
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
