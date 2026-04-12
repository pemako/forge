// Package notify provides a unified interface for sending notifications across
// multiple channels: Email (SMTP), SMS (Alibaba Cloud Dysms), DingTalk robot
// webhook, WeChat Work (WeCom) robot webhook, and Feishu (Lark) robot webhook.
//
// Create a Notifier with New, registering one or more channels via
// WithEmail, WithSMS, WithDingTalk, WithWeCom, or WithFeishu. Then call
// Send with a Message to dispatch notifications.
//
// The Channels field of Message controls which channels are used for a given
// send. When Channels is empty, the message is dispatched through every
// registered channel. Send returns the first non-nil error and stops
// further dispatch.
//
// Example:
//
//	n, err := notify.New(
//	    notify.WithEmail(notify.EmailConfig{
//	        Host:     "smtp.example.com",
//	        Port:     465,
//	        Username: "no-reply@example.com",
//	        Password: "secret",
//	    }),
//	    notify.WithDingTalk("https://oapi.dingtalk.com/robot/send?access_token=xxx"),
//	    notify.WithFeishu("https://open.feishu.cn/open-apis/bot/v2/hook/xxx"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Send to specific channels only.
//	err = n.Send(ctx, notify.Message{
//	    To:       []string{"alice@example.com"},
//	    Subject:  "Deployment succeeded",
//	    Body:     "## Deploy\nRelease v1.2.3 is live.",
//	    Channels: []notify.Channel{notify.Email, notify.DingTalk},
//	})
//
//	// Send to all registered channels.
//	err = n.Send(ctx, notify.Message{
//	    Body: "Critical alert: disk usage above 90%",
//	})
package notify

import (
	"context"
)

// Channel identifies a notification channel registered with a Notifier.
// Use the predefined constants (Email, SMS, DingTalk, WeCom, Feishu) in the
// Message.Channels field to target specific channels.
type Channel string

const (
	// Email sends the message via SMTP. Requires WithEmail configuration.
	// Uses Message.To, Message.Subject, and Message.Body (HTML supported).
	Email Channel = "email"
	// SMS sends the message via Alibaba Cloud Dysms API. Requires WithSMS
	// configuration. Uses Message.To (phone numbers) and Message.Body
	// (template parameters JSON).
	SMS Channel = "sms"
	// DingTalk sends the message to a DingTalk robot webhook. Requires
	// WithDingTalk configuration. Uses Message.Subject and Message.Body
	// (markdown). See https://open.dingtalk.com/document/robots/custom-robot-access.
	DingTalk Channel = "dingtalk"
	// WeCom sends the message to a WeChat Work robot webhook. Requires
	// WithWeCom configuration. Uses Message.Body (markdown).
	// See https://developer.work.weixin.qq.com/document/path/91770.
	WeCom Channel = "wecom"
	// Feishu sends the message to a Feishu (Lark) robot webhook. Requires
	// WithFeishu configuration. Uses Message.Body (plain text).
	// See https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot.
	Feishu Channel = "feishu"
)

// Message is the notification payload passed to Notifier.Send.
// Which fields are consumed depends on the target Channel:
//
//   - Email:            To, Subject, Body (HTML body)
//   - SMS:              To (E.164 phone numbers), Body (template params JSON)
//   - DingTalk:         Subject (card title), Body (markdown text)
//   - WeCom:            Body (markdown text)
//   - Feishu:           Body (plain text)
//
// Channels lists the channels to use for this send. When empty, the message
// is dispatched through all channels registered with the Notifier.
type Message struct {
	// To is the list of recipients (email addresses or phone numbers).
	// Ignored by webhook channels (DingTalk, WeCom, Feishu).
	To []string
	// Subject is the message subject line. Used by the Email and DingTalk
	// (card title) channels; ignored by others.
	Subject string
	// Body is the message content. Markdown is rendered by DingTalk and WeCom;
	// Feishu receives plain text; Email uses it as the HTML body.
	Body string
	// Channels specifies which channels to use for this send. All registered
	// channels are used when Channels is nil or empty.
	Channels []Channel
}

// Notifier sends Messages through one or more configured channels.
// Create one with New; the zero value is not usable.
type Notifier struct {
	senders map[Channel]sender
}

type sender interface {
	send(ctx context.Context, msg Message) error
}

type options struct {
	email    *EmailConfig
	sms      *SMSConfig
	dingtalk string // webhook URL
	wecom    string // webhook URL
	feishu   string // webhook URL
}

// Option configures a Notifier at construction time.
type Option func(*options)

// New creates a Notifier with the channels configured by opts.
// At least one channel option (WithEmail, WithSMS, WithDingTalk, WithWeCom,
// or WithFeishu) should be provided; a Notifier with no channels registered
// will silently do nothing on Send.
func New(opts ...Option) (*Notifier, error) {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	n := &Notifier{senders: make(map[Channel]sender)}
	if o.email != nil {
		n.senders[Email] = newEmailSender(*o.email)
	}
	if o.sms != nil {
		n.senders[SMS] = newSMSSender(*o.sms)
	}
	if o.dingtalk != "" {
		n.senders[DingTalk] = newWebhookSender(o.dingtalk, formatDingTalk)
	}
	if o.wecom != "" {
		n.senders[WeCom] = newWebhookSender(o.wecom, formatWeCom)
	}
	if o.feishu != "" {
		n.senders[Feishu] = newWebhookSender(o.feishu, formatFeishu)
	}
	return n, nil
}

// Send dispatches msg through the channels listed in msg.Channels.
// If msg.Channels is empty, the message is sent through all channels
// registered with this Notifier. Channels that are not registered are
// silently skipped. Send returns the first non-nil error encountered and
// stops further delivery; subsequent channels in the list are not attempted.
func (n *Notifier) Send(ctx context.Context, msg Message) error {
	channels := msg.Channels
	if len(channels) == 0 {
		for ch := range n.senders {
			channels = append(channels, ch)
		}
	}
	for _, ch := range channels {
		s, ok := n.senders[ch]
		if !ok {
			continue
		}
		if err := s.send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
