// Package cache provides a unified cache abstraction with in-memory and Redis
// backends. Backends are grouped in a Manager; multiple named Managers can be
// registered for global access.
//
// # Instance-first usage (recommended)
//
//	mgr, err := cache.New(cache.WithMemory("default"), cache.WithRedis("redis", addr))
//	cache.Register("default", mgr)        // optional global access
//	c := mgr.Client("default")            // instance access
//
// # Convenience access (small services)
//
//	c := cache.Client("default")          // delegates to Get("default").Client("default")
package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	mkerrors "github.com/pemako/forge/errors"
	"github.com/pemako/forge/logger"
)

// ErrCacheMiss is returned when a key does not exist or has expired.
var ErrCacheMiss = errors.New("cache miss")

// Cache is the common interface for all cache backends.
//
// # Portable operations
//
// Get, Set (with TTL), Del, and Exists are supported by all backends.
// Code using only these methods can switch between memory and Redis without
// change.
//
// # TTL precision
//
// TTL enforcement precision differs by backend:
//   - Redis: exact expiry, enforced server-side to the millisecond.
//   - Memory (go-cache): keys are lazily evicted during a background sweep
//     (interval set at construction). A key with TTL=1s may survive up to
//     sweep-interval longer. Do not rely on sub-second or exact TTL semantics
//     with the memory backend.
//
// A zero TTL means no expiry on both backends.
//
// # Non-portable operations
//
// Redis-specific operations (atomic increments, pub/sub, streams, Lua scripts,
// sorted sets) have no equivalent in the memory backend. Access them via
// cache.RedisClient("name") rather than the Cache interface.
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

// ErrAlreadyRegistered is returned by Register when a Manager with the given
// name already exists in the global registry.
var ErrAlreadyRegistered = mkerrors.Define(409, "CACHE_ALREADY_REGISTERED", "cache backend already registered")

// Manager owns a set of named cache backends and exposes them by name.
// Use New to create a Manager, then Register to add it to the global registry.
type Manager struct {
	backends map[string]Cache
}

// New builds all configured cache backends and returns a Manager that owns them.
// Returns an error if any Redis backend fails to connect.
func New(opts ...Option) (*Manager, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	if o.logger == nil {
		o.logger = logger.Default()
	}

	backends := make(map[string]Cache, len(o.backends))
	for _, e := range o.backends {
		var c Cache
		var err error
		switch e.backend {
		case "redis":
			c, err = newRedisCache(e.name, e.addr, e.password, e.db, o.logger)
			if err != nil {
				// Close any already-built backends before returning.
				for _, bc := range backends {
					bc.Close() //nolint:errcheck
				}
				return nil, err
			}
		default: // "memory" or empty
			c = newMemoryCache()
		}
		backends[e.name] = c
	}
	return &Manager{backends: backends}, nil
}

// Client returns the Cache for the named backend, or nil if not found.
func (m *Manager) Client(name string) Cache {
	return m.backends[name]
}

// RedisClient returns the underlying *redis.Client for the named Redis backend.
// Returns nil if the backend is not registered or is not a Redis backend.
func (m *Manager) RedisClient(name string) *redis.Client {
	c, ok := m.backends[name]
	if !ok {
		return nil
	}
	if rc, ok := c.(*redisCache); ok {
		return rc.client
	}
	return nil
}

// CloseBackend closes the named backend (named to avoid conflict with Close()).
// The backend is removed from the Manager after closing.
// Returns nil if the backend does not exist.
func (m *Manager) CloseBackend(name string) error {
	c, ok := m.backends[name]
	if !ok {
		return nil
	}
	delete(m.backends, name)
	return c.Close()
}

// CloseAll closes every backend in this Manager and clears the internal map.
func (m *Manager) CloseAll() error {
	var first error
	for name, c := range m.backends {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
		delete(m.backends, name)
	}
	return first
}

// Close calls CloseAll. It implements app.Closer.
func (m *Manager) Close() error {
	return m.CloseAll()
}

// --- Package-level registry of named *Manager instances ---

var (
	registry   = map[string]*Manager{}
	registryMu sync.RWMutex
)

// Register adds m to the global registry under name.
// Returns ErrAlreadyRegistered if a Manager with that name already exists.
func Register(name string, m *Manager) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		return ErrAlreadyRegistered
	}
	registry[name] = m
	return nil
}

// Replace stores m in the global registry under name, closing and replacing
// any previously registered Manager.
func Replace(name string, m *Manager) error {
	registryMu.Lock()
	old := registry[name]
	registry[name] = m
	registryMu.Unlock()
	if old != nil {
		old.CloseAll() //nolint:errcheck
	}
	return nil
}

// Get returns the *Manager registered under name, or nil if not found.
func Get(name string) *Manager {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[name]
}

// Client returns the Cache for the named backend from the "default" Manager.
// Returns nil if the "default" Manager is not registered or has no such backend.
func Client(name string) Cache {
	m := Get("default")
	if m == nil {
		return nil
	}
	return m.Client(name)
}

// RedisClient returns the *redis.Client for the named backend from the "default" Manager.
// Returns nil if the "default" Manager is not registered or has no Redis backend with that name.
func RedisClient(name string) *redis.Client {
	m := Get("default")
	if m == nil {
		return nil
	}
	return m.RedisClient(name)
}

// CloseAll closes all Managers in the global registry.
// Call in app.WithAfterStop to release Redis connections on shutdown.
func CloseAll() error {
	registryMu.Lock()
	defer registryMu.Unlock()
	var first error
	for name, m := range registry {
		if err := m.CloseAll(); err != nil && first == nil {
			first = err
		}
		delete(registry, name)
	}
	return first
}
