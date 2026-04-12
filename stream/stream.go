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

var (
	rdb  *redis.Client
	opts initOptions
	mu   sync.RWMutex
	subs []subscription
)

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
	opts = defaultInitOptions()
	for _, opt := range options {
		opt(&opts)
	}
	rdb = client
}

// Publish writes payload to the Redis Stream identified by topic.
// payload is JSON-marshalled before writing. The stream is trimmed
// approximately to the MaxLen configured in Init (default 10 000).
func Publish(ctx context.Context, topic string, payload any) error {
	if rdb == nil {
		return fmt.Errorf("stream: not initialized — call stream.Init first")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("stream: marshal payload: %w", err)
	}
	return rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		MaxLen: opts.maxLen,
		Approx: true,
		Values: map[string]any{"data": string(data)},
	}).Err()
}

// Subscribe registers a handler for the given topic.
// Must be called before Worker().Start — subscriptions registered after Start
// are not picked up by the running Worker.
//
// Use WithGroup for work-queue delivery (one instance processes each message).
// Use WithBroadcast for fan-out delivery (every instance receives each message).
func Subscribe(topic string, opts ...SubOption) {
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

	mu.Lock()
	subs = append(subs, s)
	mu.Unlock()
}

// ResetSubs clears all registered subscriptions.
// Intended for use in tests only to isolate test cases that share global state.
func ResetSubs() {
	mu.Lock()
	subs = subs[:0]
	mu.Unlock()
}

// Worker returns a core.Server that reads from all registered subscriptions.
// Add it to app.WithServer to integrate with the application lifecycle.
// Worker.Start blocks until Worker.Stop is called.
func Worker(l ...core.Logger) *streamWorker {
	log := opts.log
	if len(l) > 0 && l[0] != nil {
		log = l[0]
	}
	return &streamWorker{
		done: make(chan struct{}),
		log:  log,
	}
}
