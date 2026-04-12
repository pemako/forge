package notify

import (
	"context"
	"fmt"

	"gopkg.in/gomail.v2"
)

type emailSender struct {
	cfg EmailConfig
}

func newEmailSender(cfg EmailConfig) *emailSender {
	if cfg.From == "" {
		cfg.From = cfg.Username
	}
	return &emailSender{cfg: cfg}
}

func (s *emailSender) send(_ context.Context, msg Message) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.cfg.From)
	m.SetHeader("To", msg.To...)
	m.SetHeader("Subject", msg.Subject)
	m.SetBody("text/html", msg.Body)

	d := gomail.NewDialer(s.cfg.Host, s.cfg.Port, s.cfg.Username, s.cfg.Password)
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("notify/email: send to %v: %w", msg.To, err)
	}
	return nil
}
