package stream

import (
	"os"
	"time"

	"github.com/pemako/mk/core"
	"github.com/pemako/mk/logger"
)

// initOptions configures stream runtime behaviour (not the Redis connection).
type initOptions struct {
	log    core.Logger
	maxLen int64
}

// InitOption configures stream runtime behaviour passed to Init.
type InitOption func(*initOptions)

// WithLogger sets the logger for stream operations (default: logger.Default()).
func WithLogger(l core.Logger) InitOption {
	return func(o *initOptions) { o.log = l }
}

// WithMaxLen sets the approximate maximum number of entries retained per
// stream via MAXLEN trimming (default 10 000).
func WithMaxLen(n int64) InitOption {
	return func(o *initOptions) { o.maxLen = n }
}

func defaultInitOptions() initOptions {
	return initOptions{log: logger.Default(), maxLen: 10_000}
}

// subscription holds the configuration for one Subscribe call.
type subscription struct {
	topic       string
	group       string
	consumer    string
	handler     HandlerFunc
	retries     int
	retryDelay  time.Duration
	concurrency int
	broadcast   bool
}

// SubOption configures a single Subscribe call.
type SubOption func(*subscription)

// WithGroup sets the Redis consumer group name.
// All instances sharing the same group name form a work queue: each message
// is delivered to exactly one instance.
func WithGroup(group string) SubOption {
	return func(s *subscription) { s.group = group }
}

// WithBroadcast causes every instance to receive each message independently.
// Internally each instance joins a unique consumer group derived from its
// hostname, so Redis delivers the message to all groups.
// WithBroadcast and WithGroup are mutually exclusive; WithBroadcast takes
// precedence.
func WithBroadcast() SubOption {
	return func(s *subscription) { s.broadcast = true }
}

// WithConsumer sets the consumer name within the group (default: hostname).
// Must be unique per instance within the same group.
func WithConsumer(name string) SubOption {
	return func(s *subscription) { s.consumer = name }
}

// WithHandler sets the message processing function. Required.
func WithHandler(fn HandlerFunc) SubOption {
	return func(s *subscription) { s.handler = fn }
}

// WithRetries sets the maximum number of processing retries before a message
// is moved to the dead-letter stream (<topic>:dead). Default is 0.
func WithRetries(n int) SubOption {
	return func(s *subscription) { s.retries = n }
}

// WithRetryDelay sets the base delay between retries (default 1s).
func WithRetryDelay(d time.Duration) SubOption {
	return func(s *subscription) { s.retryDelay = d }
}

// WithConcurrency sets the number of goroutines reading from this
// subscription concurrently (default 1).
func WithConcurrency(n int) SubOption {
	return func(s *subscription) { s.concurrency = n }
}

func defaultSubOptions(topic string) subscription {
	h, _ := os.Hostname()
	if h == "" {
		h = "unknown"
	}
	return subscription{
		topic:       topic,
		consumer:    h,
		retryDelay:  time.Second,
		concurrency: 1,
	}
}
