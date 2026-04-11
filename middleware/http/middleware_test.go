package httpMiddleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/pemako/mk/middleware/http"
	"github.com/stretchr/testify/assert"
)

func newTestRouter(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handlers...)
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func TestTimeout_cancels(t *testing.T) {
	r := gin.New()
	r.Use(mw.Timeout(20 * time.Millisecond))
	r.GET("/slow", func(c *gin.Context) {
		select {
		case <-c.Request.Context().Done():
			// Context cancelled — return without writing so middleware writes 503
			return
		case <-time.After(200 * time.Millisecond):
			c.String(200, "ok")
		}
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/slow", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 503, w.Code)
}

func TestBodyLimit_rejects(t *testing.T) {
	r := gin.New()
	r.Use(mw.BodyLimit("5B"))
	r.POST("/upload", func(c *gin.Context) {
		buf := make([]byte, 100)
		_, err := c.Request.Body.Read(buf)
		if err != nil && strings.Contains(err.Error(), "http: request body too large") {
			c.JSON(413, gin.H{"error": "too large"})
			return
		}
		c.String(200, "ok")
	})
	body := strings.NewReader("hello world this is too big")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/upload", body)
	r.ServeHTTP(w, req)
	assert.Equal(t, 413, w.Code)
}

func TestRateLimit_blocks(t *testing.T) {
	r := newTestRouter(mw.RateLimit(mw.RateLimitConfig{Rate: 1, Burst: 1}))
	do := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, 200, do())
	assert.Equal(t, 429, do())
}

func TestIPFilter_whitelist_allows(t *testing.T) {
	r := newTestRouter(mw.IPFilter(mw.IPFilterConfig{Mode: "whitelist", IPs: []string{"127.0.0.1"}}))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestIPFilter_whitelist_blocks(t *testing.T) {
	r := newTestRouter(mw.IPFilter(mw.IPFilterConfig{Mode: "whitelist", IPs: []string{"127.0.0.1"}}))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	r.ServeHTTP(w, req)
	assert.Equal(t, 403, w.Code)
}
