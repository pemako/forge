package core

import "context"

// HealthChecker reports the health of a named component.
// Implementations are provided by infrastructure packages (e.g. database.Manager,
// cache.Manager) and consumed by server/http and app for readiness endpoints.
//
// A non-nil error means the component is not ready. The error message may be
// included in the /readyz response.
type HealthChecker interface {
	// Name returns a short identifier for this check (e.g. "database:default",
	// "cache:redis"). Used as the key in the /readyz response body.
	Name() string
	// Check performs a lightweight health probe. It should complete quickly
	// (within the readiness probe timeout) and return a descriptive error
	// if the component is unhealthy.
	Check(ctx context.Context) error
}
