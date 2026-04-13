package database

import (
	"context"
	"fmt"

	"github.com/pemako/mk/core"
)

type dbHealthChecker struct {
	mgr      *Manager
	connName string
}

// HealthChecker returns a core.HealthChecker that probes the named database
// connection with a lightweight SELECT 1 query.
func (m *Manager) HealthChecker(connName string) core.HealthChecker {
	return &dbHealthChecker{mgr: m, connName: connName}
}

func (c *dbHealthChecker) Name() string { return "database:" + c.connName }

func (c *dbHealthChecker) Check(ctx context.Context) error {
	db := c.mgr.DB(c.connName)
	if db == nil {
		return fmt.Errorf("connection %q not registered", c.connName)
	}
	return db.WithContext(ctx).Exec("SELECT 1").Error
}
