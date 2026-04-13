// Package ratelimit provides a standalone rate limiter that can be used
// directly in service or repository code, independent of HTTP or gRPC middleware.
//
// Two backends are available:
//
//   - In-process (default): token-bucket algorithm backed by golang.org/x/time/rate.
//     Each unique key gets its own bucket. Suitable for single-instance services.
//
//   - Distributed: fixed-window counter stored in Redis via an atomic Lua script.
//     All instances share the same counters, so limits are enforced cluster-wide.
//     Requires a *redis.Client, typically obtained from cache.RedisClient.
//
// Example (in-process):
//
//	lim := ratelimit.New(ratelimit.WithRate(100), ratelimit.WithBurst(10))
//	if !lim.Allow(ctx, userID) {
//	    return errors.Wrap(errors.ErrTooMany, "user %s", userID)
//	}
//
// Example (distributed):
//
//	lim := ratelimit.New(
//	    ratelimit.WithRate(100),
//	    ratelimit.WithRedis(cache.RedisClient("redis")),
//	)
//	if !lim.Allow(ctx, userID) { ... }
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Limiter enforces a rate limit per string key.
// All implementations are safe for concurrent use.
type Limiter interface {
	// Allow reports whether one event for the given key is permitted right now.
	// Returns false if the rate limit has been exceeded.
	Allow(ctx context.Context, key string) bool
}

// New creates a Limiter with the given options.
// When WithRedis is provided, a distributed Redis-backed limiter is returned;
// otherwise an in-process token-bucket limiter is used.
func New(opts ...Option) Limiter {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if o.rdb != nil {
		return &redisLimiter{
			rdb:    o.rdb,
			rate:   o.rate,
			window: o.window,
		}
	}
	return &inProcessLimiter{
		rate:  rate.Limit(o.rate),
		burst: o.burst,
	}
}

// --- in-process token-bucket limiter ---

type inProcessLimiter struct {
	rate  rate.Limit
	burst int
	mu    sync.Map // key → *rate.Limiter
}

func (l *inProcessLimiter) Allow(_ context.Context, key string) bool {
	v, _ := l.mu.LoadOrStore(key, rate.NewLimiter(l.rate, l.burst))
	return v.(*rate.Limiter).Allow()
}

// --- distributed Redis fixed-window limiter ---

// slidingWindowLua implements a fixed-window counter using Redis.
// KEYS[1] = rate limit key, ARGV[1] = window in seconds, ARGV[2] = max requests.
// Returns 1 if allowed, 0 if denied.
const fixedWindowLua = `
local key    = KEYS[1]
local window = tonumber(ARGV[1])
local limit  = tonumber(ARGV[2])
local count  = redis.call("INCR", key)
if count == 1 then
    redis.call("EXPIRE", key, window)
end
if count > limit then
    return 0
end
return 1
`

type redisLimiter struct {
	rdb    *redis.Client
	rate   float64       // max requests per window
	window time.Duration // window size; defaults to 1s
}

func (l *redisLimiter) Allow(ctx context.Context, key string) bool {
	window := l.window
	if window <= 0 {
		window = time.Second
	}
	windowSecs := max(int64(window.Seconds()), 1)
	result, err := l.rdb.Eval(ctx, fixedWindowLua,
		[]string{key},
		windowSecs,
		int64(l.rate),
	).Int()
	if err != nil {
		// On Redis error, fail open (allow the request).
		return true
	}
	return result == 1
}
