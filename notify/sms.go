package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type smsSender struct {
	cfg SMSConfig
}

func newSMSSender(cfg SMSConfig) *smsSender {
	if cfg.Region == "" {
		cfg.Region = "cn-hangzhou"
	}
	return &smsSender{cfg: cfg}
}

func (s *smsSender) send(_ context.Context, msg Message) error {
	for _, phone := range msg.To {
		if err := s.sendOne(phone, msg.Body); err != nil {
			return err
		}
	}
	return nil
}

func (s *smsSender) sendOne(phone, templateParam string) error {
	params := map[string]string{
		"AccessKeyId":      s.cfg.AccessKey,
		"Action":           "SendSms",
		"Format":           "JSON",
		"PhoneNumbers":     phone,
		"RegionId":         s.cfg.Region,
		"SignName":         s.cfg.SignName,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"SignatureVersion": "1.0",
		"TemplateCode":     s.cfg.TemplateCode,
		"TemplateParam":    templateParam,
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Version":          "2017-05-25",
	}

	// Build canonical query string
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	query := strings.Join(parts, "&")
	toSign := "GET&%2F&" + url.QueryEscape(query)

	mac := hmac.New(sha256.New, []byte(s.cfg.SecretKey+"&"))
	mac.Write([]byte(toSign))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	finalURL := "https://dysmsapi.aliyuncs.com/?" + query + "&Signature=" + url.QueryEscape(sig)

	resp, err := http.Get(finalURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("notify/sms: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("notify/sms: parse response: %w", err)
	}
	if result.Code != "OK" {
		return fmt.Errorf("notify/sms: api error: %s - %s", result.Code, result.Message)
	}
	return nil
}
