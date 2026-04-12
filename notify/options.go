package notify

// EmailConfig holds the SMTP connection settings for the Email channel.
// Port 465 uses implicit TLS (SMTPS); port 587 uses STARTTLS.
type EmailConfig struct {
	// Host is the SMTP server hostname (e.g., "smtp.gmail.com").
	Host string
	// Port is the SMTP server port (commonly 465 for SMTPS or 587 for STARTTLS).
	Port int
	// Username is the SMTP authentication user and the default sender address.
	Username string
	// Password is the SMTP authentication credential (application password for
	// providers that require two-factor authentication).
	Password string
	// From is the "From" address in outgoing messages.
	// Defaults to Username when empty.
	From string
}

// SMSConfig holds the Alibaba Cloud Dysms configuration for the SMS channel.
// Obtain the SignName and TemplateCode from the Alibaba Cloud Dysms console.
type SMSConfig struct {
	// AccessKey is the Alibaba Cloud RAM access key ID.
	AccessKey string
	// SecretKey is the Alibaba Cloud RAM access key secret.
	SecretKey string
	// SignName is the SMS signature name registered in the Dysms console.
	SignName string
	// TemplateCode is the SMS template code registered in the Dysms console.
	TemplateCode string
	// Region is the Dysms API region endpoint. Defaults to "cn-hangzhou".
	Region string
}

// WithEmail registers the Email channel using the provided SMTP configuration.
// The Email channel uses Message.To, Message.Subject, and Message.Body (HTML).
func WithEmail(cfg EmailConfig) Option {
	return func(o *options) { o.email = &cfg }
}

// WithSMS registers the SMS channel using the Alibaba Cloud Dysms API.
// The SMS channel uses Message.To (phone numbers in E.164 format) and
// Message.Body (JSON string of template variable values, e.g.,
// `{"code":"1234"}`).
func WithSMS(cfg SMSConfig) Option {
	return func(o *options) { o.sms = &cfg }
}

// WithDingTalk registers the DingTalk channel using the given robot webhook URL.
// The webhook URL is obtained from the DingTalk group robot settings.
// The DingTalk channel uses Message.Subject (card title) and Message.Body
// (markdown content).
func WithDingTalk(webhookURL string) Option {
	return func(o *options) { o.dingtalk = webhookURL }
}

// WithWeCom registers the WeChat Work channel using the given robot webhook URL.
// The webhook URL is obtained from the WeCom group robot settings.
// The WeCom channel uses Message.Body (markdown content).
func WithWeCom(webhookURL string) Option {
	return func(o *options) { o.wecom = webhookURL }
}

// WithFeishu registers the Feishu (Lark) channel using the given robot webhook
// URL. The webhook URL is obtained from the Feishu group bot settings.
// The Feishu channel uses Message.Body (plain text).
func WithFeishu(webhookURL string) Option {
	return func(o *options) { o.feishu = webhookURL }
}
