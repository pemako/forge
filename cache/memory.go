package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type memEntry struct {
	value     string
	expiresAt time.Time // zero = no expiry
}

func (e memEntry) expired() bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}

type memoryCache struct {
	mu      sync.RWMutex
	entries map[string]memEntry
}

func newMemoryCache() *memoryCache {
	c := &memoryCache{entries: make(map[string]memEntry)}
	go c.cleanup()
	return c
}

// cleanup removes expired keys every minute.
func (c *memoryCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		for k, e := range c.entries {
			if e.expired() {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}

func (c *memoryCache) Get(_ context.Context, key string) (string, error) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || e.expired() {
		return "", ErrCacheMiss
	}
	return e.value, nil
}

func (c *memoryCache) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		s = fmt.Sprintf("%v", v)
	}
	entry := memEntry{value: s}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	for _, k := range keys {
		delete(c.entries, k)
	}
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Get(ctx, key)
	if err == ErrCacheMiss {
		return false, nil
	}
	return err == nil, err
}

// Close is a no-op for the in-memory backend.
func (c *memoryCache) Close() error { return nil }
