package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/pemako/mk/logger"
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
		registryMu.Lock()
		registry[e.name] = c
		registryMu.Unlock()
	}
	return nil
}

// Client returns the Cache for the named instance, or nil if not registered.
func Client(name string) Cache {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}
