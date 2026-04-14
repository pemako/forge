package ratelimit

import (
	"time"

	"github.com/pemako/forge/core"
	"github.com/pemako/forge/logger"
	"github.com/redis/go-redis/v9"
)

type options struct {
	rate   float64
	burst  int
	window time.Duration
	rdb    *redis.Client
	log    core.Logger
}

// Option configures a Limiter.
type Option func(*options)

func defaultOptions() options {
	return options{
		rate:   10,
		burst:  10,
		window: time.Second,
		log:    logger.Default(),
	}
}

// WithRate sets the sustained request rate (requests per window).
// For in-process limiters, this is requests per second (token-bucket rate).
// For distributed limiters, this is max requests per WithWindow duration.
// Defaults to 10.
func WithRate(r float64) Option {
	return func(o *options) { o.rate = r }
}

// WithBurst sets the maximum burst size for the in-process token-bucket limiter.
// Ignored by the distributed limiter. Defaults to 10.
func WithBurst(b int) Option {
	return func(o *options) { o.burst = b }
}

// WithWindow sets the fixed time window for the distributed Redis limiter.
// Ignored by the in-process limiter. Defaults to 1 second.
func WithWindow(d time.Duration) Option {
	return func(o *options) { o.window = d }
}

// WithRedis enables the distributed Redis-backed limiter using the provided client.
// Typically obtained via cache.RedisClient("name").
// When this option is set, WithBurst has no effect.
func WithRedis(rdb *redis.Client) Option {
	return func(o *options) { o.rdb = rdb }
}

// WithLogger sets the logger. Defaults to logger.Default().
func WithLogger(l core.Logger) Option {
	return func(o *options) { o.log = l }
}

// WithConfig reads ratelimit settings from cfg:
//
//	ratelimit.rate   — max requests (float64)
//	ratelimit.burst  — burst size for in-process limiter (int)
//	ratelimit.window — time window for distributed limiter (duration, e.g. "1s")
func WithConfig(cfg core.Config) Option {
	return func(o *options) {
		if v := cfg.GetFloat64("ratelimit.rate"); v > 0 {
			o.rate = v
		}
		if v := cfg.GetInt("ratelimit.burst"); v > 0 {
			o.burst = v
		}
		if v := cfg.GetDuration("ratelimit.window"); v > 0 {
			o.window = v
		}
	}
}
