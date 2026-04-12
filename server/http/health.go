package httpserver

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// readyFlag tracks whether the application has completed startup and is ready
// to serve production traffic. It is false by default so that /readyz returns
// 503 until explicitly set to true.
var readyFlag atomic.Bool

// SetReady updates the readiness state of the service. Call SetReady(true) once
// all startup work (e.g., migrations, cache warm-up) has completed so that
// the /readyz endpoint starts returning HTTP 200. Call SetReady(false) during
// planned maintenance to temporarily remove the instance from load balancer rotation.
func SetReady(ready bool) { readyFlag.Store(ready) }

func healthzHandler(svcName, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": svcName,
			"version": version,
		})
	}
}

func readyzHandler(svcName, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !readyFlag.Load() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": svcName,
			"version": version,
		})
	}
}
