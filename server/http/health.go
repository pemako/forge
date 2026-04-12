package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// readyzHandler returns a handler that reads the ready flag from the server instance.
func readyzHandler(svcName, version string, ready func() bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ready() {
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

func healthzHandler(svcName, version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": svcName,
			"version": version,
		})
	}
}
