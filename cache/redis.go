package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/pemako/mk/core"
)

type redisCache struct {
	name   string
	client *redis.Client
}

func newRedisCache(name, addr, password string, db int, log core.Logger) (*redisCache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		// Force RESP2 to avoid a go-redis v9 RESP3 compatibility issue where
		// XREADGROUP BLOCK returns a null type that the RESP3 reader does not
		// handle correctly, causing the read to hang until the TCP deadline.
		Protocol: 2,
		// Respect context cancellation so stream workers can interrupt blocking
		// reads (XREADGROUP BLOCK 0) cleanly when the app shuts down.
		ContextTimeoutEnabled: true,
	})
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := rdb.Ping(pingCtx).Err()
	cancel()
	if err != nil {
		return nil, fmt.Errorf("redis %q ping: %w", name, err)
	}
	rdb.AddHook(newLogHook(name, log))
	return &redisCache{name: name, client: rdb}, nil
}

func (r *redisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	return val, err
}

func (r *redisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

func (r *redisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}

// Close closes the underlying Redis connection pool.
func (r *redisCache) Close() error { return r.client.Close() }

type logHook struct {
	name string
	log  core.Logger
}

func newLogHook(name string, log core.Logger) *logHook {
	return &logHook{name: name, log: log}
}

func (h *logHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *logHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			h.log.Warn(ctx, "slow redis command",
				core.Field{Key: "redis", Value: h.name},
				core.Field{Key: "cmd", Value: cmd.Name()},
				core.Field{Key: "elapsed", Value: elapsed.String()},
			)
		}
		return err
	}
}

func (h *logHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
