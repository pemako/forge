// httpclient/client_test.go
package httpclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pemako/mk/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackoffDelay_increases(t *testing.T) {
	d1 := httpclient.ExportedBackoffDelay(1, 100*time.Millisecond, 2*time.Second)
	d2 := httpclient.ExportedBackoffDelay(2, 100*time.Millisecond, 2*time.Second)
	d3 := httpclient.ExportedBackoffDelay(3, 100*time.Millisecond, 2*time.Second)
	assert.Less(t, d1, d2)
	assert.Less(t, d2, d3)
}

func TestBackoffDelay_capped(t *testing.T) {
	// With result capped at maxDelay, the output must never exceed maxDelay.
	for range 100 {
		d := httpclient.ExportedBackoffDelay(10, 100*time.Millisecond, 500*time.Millisecond)
		assert.LessOrEqual(t, d, 500*time.Millisecond)
	}
}

func TestClient_Get_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		json.NewEncoder(w).Encode(map[string]string{"name": "alice"})
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.WithBaseURL(srv.URL))
	var out map[string]string
	err := client.Get(context.Background(), "/", &out)
	require.NoError(t, err)
	assert.Equal(t, "alice", out["name"])
}

func TestClient_Post_sendsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(body, &m)
		assert.Equal(t, "bob", m["name"])
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(m)
	}))
	defer srv.Close()

	client := httpclient.New(httpclient.WithBaseURL(srv.URL))
	var out map[string]string
	err := client.Post(context.Background(), "/", map[string]string{"name": "bob"}, &out)
	require.NoError(t, err)
	assert.Equal(t, "bob", out["name"])
}

func TestClient_Retry_on5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"ok": "yes"})
	}))
	defer srv.Close()

	client := httpclient.New(
		httpclient.WithBaseURL(srv.URL),
		httpclient.WithRetry(3),
		httpclient.WithRetryBaseDelay(5*time.Millisecond),
	)
	var out map[string]string
	err := client.Get(context.Background(), "/", &out)
	require.NoError(t, err)
	assert.Equal(t, int32(3), calls.Load())
}

func TestClient_NoRetry_on4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := httpclient.New(
		httpclient.WithBaseURL(srv.URL),
		httpclient.WithRetry(3),
	)
	err := client.Get(context.Background(), "/", nil)
	assert.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "4xx must not retry")
}

func TestRegistry_RegisterGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"reg": "ok"})
	}))
	defer srv.Close()

	httpclient.Register("reg_test", httpclient.WithBaseURL(srv.URL))
	c := httpclient.Get("reg_test")
	require.NotNil(t, c)
	var out map[string]string
	require.NoError(t, c.Get(context.Background(), "/", &out))
	assert.Equal(t, "ok", out["reg"])
}

func TestGet_unknownName(t *testing.T) {
	assert.Nil(t, httpclient.Get("nonexistent_xyz_http"))
}
