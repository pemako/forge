package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type webhookSender struct {
	url    string
	format func(msg Message) ([]byte, error)
}

func newWebhookSender(webhookURL string, format func(Message) ([]byte, error)) *webhookSender {
	return &webhookSender{url: webhookURL, format: format}
}

func (s *webhookSender) send(ctx context.Context, msg Message) error {
	payload, err := s.format(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify/webhook: post to %s: %w", s.url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify/webhook: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// formatDingTalk builds a DingTalk robot markdown message payload.
// msg.Subject is used as the card title; msg.Body is the markdown content.
func formatDingTalk(msg Message) ([]byte, error) {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": msg.Subject,
			"text":  msg.Body,
		},
	}
	return json.Marshal(payload)
}

// formatWeCom builds a WeChat Work robot markdown message payload.
// msg.Body is used as the markdown content.
func formatWeCom(msg Message) ([]byte, error) {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": msg.Body,
		},
	}
	return json.Marshal(payload)
}

// formatFeishu builds a Feishu (Lark) robot text message payload.
// msg.Body is sent as plain text content.
func formatFeishu(msg Message) ([]byte, error) {
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": msg.Body,
		},
	}
	return json.Marshal(payload)
}
