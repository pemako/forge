// Package cache provides a unified caching abstraction with in-memory and
// Redis backends. The in-memory backend is the default and requires no
// external dependencies. The Redis backend is optional and configured via
// WithRedis or WithConfig.
//
// A package-level registry stores named Cache instances. All instances that
// need Redis share the connection through cache.RedisClient, which is also
// used by the lock and stream packages.
//
// Example:
//
//	cache.Init(
//	    cache.WithMemory("default"),
//	    cache.WithRedis("redis", "redis:6379"),
//	)
//	c := cache.Client("default")
//	c.Set(ctx, "key", "value", time.Hour)
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/pemako/mk/logger"
	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned when a key does not exist or has expired.
var ErrCacheMiss = errors.New("cache miss")

// Cache is the common interface for all cache backends.
type Cache interface {
	// Get retrieves the string value for key.
	// Returns ErrCacheMiss if the key does not exist or has expired.
	Get(ctx context.Context, key string) (string, error)
	// Set stores value with the given TTL. A zero TTL means no expiry.
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	// Del deletes one or more keys. Missing keys are silently ignored.
	Del(ctx context.Context, keys ...string) error
	// Exists reports whether key exists and has not expired.
	Exists(ctx context.Context, key string) (bool, error)
	// Close releases resources held by this cache backend.
	// For in-memory backends this is a no-op.
	// For Redis backends it closes the underlying connection pool.
	Close() error
}

var (
	registry   = map[string]Cache{}
	registryMu sync.RWMutex
)

// Init initializes all configured cache backends and registers them.
// Returns error if any Redis backend fails to connect.
func Init(opts ...Option) error {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	if o.logger == nil {
		o.logger = logger.Default()
	}

	// Build all backends first, then lock once for registry writes.
	built := make(map[string]Cache, len(o.backends))
	for _, e := range o.backends {
		var c Cache
		var err error
		switch e.backend {
		case "redis":
			c, err = newRedisCache(e.name, e.addr, e.password, e.db, o.logger)
			if err != nil {
				return err
			}
		default: // "memory" or empty
			c = newMemoryCache()
		}
		built[e.name] = c
	}

	registryMu.Lock()
	for name, c := range built {
		registry[name] = c
	}
	registryMu.Unlock()
	return nil
}

// Client returns the Cache for the named instance, or nil if not registered.
func Client(name string) Cache {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// RedisClient returns the underlying *redis.Client for the named Redis backend.
// Returns nil if the backend is not registered or is not a Redis backend
// (e.g., in-memory backend).
// Used by packages that need direct access to Redis operations such as
// stream processing.
func RedisClient(name string) *redis.Client {
	registryMu.RLock()
	defer registryMu.RUnlock()
	c := registry[name]
	if rc, ok := c.(*redisCache); ok {
		return rc.client
	}
	return nil
}

// Close closes the named cache backend and removes it from the registry.
func Close(name string) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	c, ok := registry[name]
	if !ok {
		return nil
	}
	delete(registry, name)
	return c.Close()
}

// CloseAll closes all registered cache backends.
// Call in app.WithAfterStop to release Redis connections on shutdown.
func CloseAll() error {
	registryMu.Lock()
	defer registryMu.Unlock()
	var first error
	for name, c := range registry {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
		delete(registry, name)
	}
	return first
}
