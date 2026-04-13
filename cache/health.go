package cache

import (
	"context"
	"fmt"

	"github.com/pemako/mk/core"
)

type cacheHealthChecker struct {
	mgr         *Manager
	backendName string
}

// HealthChecker returns a core.HealthChecker for the named cache backend.
// For memory backends, the check always succeeds (no external dependency).
// For Redis backends, the check performs a PING command.
func (m *Manager) HealthChecker(backendName string) core.HealthChecker {
	return &cacheHealthChecker{mgr: m, backendName: backendName}
}

func (c *cacheHealthChecker) Name() string { return "cache:" + c.backendName }

func (c *cacheHealthChecker) Check(ctx context.Context) error {
	backend := c.mgr.backends[c.backendName]
	if backend == nil {
		return fmt.Errorf("cache backend %q not registered", c.backendName)
	}
	// Redis backends support a PING check; memory backends are always healthy.
	if rc, ok := backend.(*redisCache); ok {
		return rc.client.Ping(ctx).Err()
	}
	return nil // memory backend is always ready
}
