package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pemako/mk/notify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_noChannels(t *testing.T) {
	n, err := notify.New()
	require.NoError(t, err)
	// No channels registered; Send should succeed (nothing to do)
	err = n.Send(context.Background(), notify.Message{Body: "hello"})
	assert.NoError(t, err)
}

func TestDingTalk_sendsPayload(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received) //nolint:errcheck
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n, err := notify.New(notify.WithDingTalk(srv.URL))
	require.NoError(t, err)

	err = n.Send(context.Background(), notify.Message{
		Subject:  "Alert",
		Body:     "Deployment done",
		Channels: []notify.Channel{notify.DingTalk},
	})
	require.NoError(t, err)
	assert.Equal(t, "markdown", received["msgtype"])
}

func TestWeCom_sendsPayload(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received) //nolint:errcheck
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n, err := notify.New(notify.WithWeCom(srv.URL))
	require.NoError(t, err)

	err = n.Send(context.Background(), notify.Message{
		Body:     "Hello WeCom",
		Channels: []notify.Channel{notify.WeCom},
	})
	require.NoError(t, err)
	assert.Equal(t, "markdown", received["msgtype"])
}

func TestFeishu_sendsPayload(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received) //nolint:errcheck
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n, err := notify.New(notify.WithFeishu(srv.URL))
	require.NoError(t, err)

	err = n.Send(context.Background(), notify.Message{
		Body:     "Hello Feishu",
		Channels: []notify.Channel{notify.Feishu},
	})
	require.NoError(t, err)
	assert.Equal(t, "text", received["msg_type"])
}

func TestSend_specificChannels(t *testing.T) {
	var dingCalled, wecomCalled bool

	ding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dingCalled = true
		w.WriteHeader(200)
	}))
	defer ding.Close()
	wecom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wecomCalled = true
		w.WriteHeader(200)
	}))
	defer wecom.Close()

	n, err := notify.New(
		notify.WithDingTalk(ding.URL),
		notify.WithWeCom(wecom.URL),
	)
	require.NoError(t, err)

	// Only send to DingTalk
	err = n.Send(context.Background(), notify.Message{
		Body:     "test",
		Channels: []notify.Channel{notify.DingTalk},
	})
	require.NoError(t, err)
	assert.True(t, dingCalled)
	assert.False(t, wecomCalled, "WeCom should not be called")
}

func TestSend_allChannels_whenEmpty(t *testing.T) {
	var dingCalled, wecomCalled bool

	ding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dingCalled = true
		w.WriteHeader(200)
	}))
	defer ding.Close()
	wecom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wecomCalled = true
		w.WriteHeader(200)
	}))
	defer wecom.Close()

	n, err := notify.New(
		notify.WithDingTalk(ding.URL),
		notify.WithWeCom(wecom.URL),
	)
	require.NoError(t, err)

	// Send with no channels specified — should call all registered
	err = n.Send(context.Background(), notify.Message{Body: "broadcast"})
	require.NoError(t, err)
	assert.True(t, dingCalled)
	assert.True(t, wecomCalled)
}

func TestWebhook_httpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error")) //nolint:errcheck
	}))
	defer srv.Close()

	n, err := notify.New(notify.WithFeishu(srv.URL))
	require.NoError(t, err)

	err = n.Send(context.Background(), notify.Message{
		Body:     "test",
		Channels: []notify.Channel{notify.Feishu},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestSMSSend_CancelledContext_ReturnsError(t *testing.T) {
	n, err := notify.New(notify.WithSMS(notify.SMSConfig{
		AccessKey:    "test-key",
		SecretKey:    "test-secret",
		SignName:     "test",
		TemplateCode: "SMS_123",
	}))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err = n.Send(ctx, notify.Message{
		To:       []string{"13800000000"},
		Body:     `{"code":"1234"}`,
		Channels: []notify.Channel{notify.SMS},
	})
	// Should fail because context is cancelled
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
