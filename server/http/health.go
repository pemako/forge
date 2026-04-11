package httpserver

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

var readyFlag atomic.Bool

// SetReady sets the readiness state. Call with true after migrations complete.
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
