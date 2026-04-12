package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// readyzHandler returns a handler that checks the ready flag AND runs all
// registered custom check functions. Returns 200 only when all pass.
// On failure it returns 503 with a JSON map of check name → "ok" or error message.
func readyzHandler(svcName, version string, ready func() bool, checks []readyCheck) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ready() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
			})
			return
		}

		if len(checks) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"status":  "ok",
				"service": svcName,
				"version": version,
			})
			return
		}

		results := make(map[string]string, len(checks))
		allOK := true
		for _, ch := range checks {
			if err := ch.fn(c.Request.Context()); err != nil {
				results[ch.name] = err.Error()
				allOK = false
			} else {
				results[ch.name] = "ok"
			}
		}

		if !allOK {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"checks": results,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": svcName,
			"version": version,
			"checks":  results,
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
