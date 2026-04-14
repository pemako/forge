package httpMiddleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/pemako/forge/errors"
	mw "github.com/pemako/forge/middleware/http"
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

func TestErrorHandler_CodedError_returns_correct_code(t *testing.T) {
	r := gin.New()
	r.Use(mw.ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		c.Error(errors.ErrNotFound) //nolint:errcheck
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 404, w.Code)
	assert.Contains(t, w.Body.String(), "NOT_FOUND")
}

func TestErrorHandler_GenericError_returns_500(t *testing.T) {
	r := gin.New()
	r.Use(mw.ErrorHandler())
	r.GET("/test", func(c *gin.Context) {
		c.Error(fmt.Errorf("something internal")) //nolint:errcheck
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
	assert.Contains(t, w.Body.String(), "INTERNAL")
}

func TestErrorHandler_NoError_passthrough(t *testing.T) {
	r := gin.New()
	r.Use(mw.ErrorHandler())
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestRequestID_generates_when_missing(t *testing.T) {
	r := gin.New()
	r.Use(mw.RequestID())
	r.GET("/test", func(c *gin.Context) { c.String(200, c.GetString("request_id")) })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.NotEmpty(t, w.Header().Get(mw.HeaderRequestID))
	assert.Equal(t, w.Header().Get(mw.HeaderRequestID), w.Body.String())
}

func TestRequestID_reuses_existing_header(t *testing.T) {
	r := gin.New()
	r.Use(mw.RequestID())
	r.GET("/test", func(c *gin.Context) { c.String(200, c.GetString("request_id")) })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set(mw.HeaderRequestID, "my-custom-id")
	r.ServeHTTP(w, req)
	assert.Equal(t, "my-custom-id", w.Header().Get(mw.HeaderRequestID))
	assert.Equal(t, "my-custom-id", w.Body.String())
}

func TestRecovery_catches_panic(t *testing.T) {
	r := gin.New()
	r.Use(mw.Recovery())
	r.GET("/panic", func(c *gin.Context) { panic("test panic") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 500, w.Code)
}

func TestCORS_preflight_returns_204(t *testing.T) {
	r := gin.New()
	r.Use(mw.CORS(mw.CORSConfig{AllowOrigins: []string{"*"}}))
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	r.ServeHTTP(w, req)
	assert.Equal(t, 204, w.Code)
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Origin"))
}
