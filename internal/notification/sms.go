package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SMSConfig mirrors VoiceConfig — see voice.go for the full rationale on
// the generic-webhook envelope and the provider/auth split.
//
// SMS shares voice's "one POST per recipient" pattern (no native
// fan-out at the provider level for the major SMS gateways), so the
// SendBatched implementation iterates Recipients sequentially and
// aggregates the first non-nil error.
type SMSConfig struct {
	ProviderURL string `json:"provider_url"`
	AuthHeader  string `json:"auth_header"`
	// SignName is required by Aliyun-SMS / TencentCloud-SMS; empty for
	// providers that don't need it (Twilio, Nexmo, Plivo).
	SignName string `json:"sign_name"`
	// TemplateCode is the provider-side approved SMS template id.
	// Required for CN providers; ignored elsewhere.
	TemplateCode string `json:"template_code"`
}

type SMSChannel struct {
	httpClient *http.Client
}

func NewSMSChannel(client *http.Client) *SMSChannel {
	if client == nil {
		client = http.DefaultClient
	}
	return &SMSChannel{httpClient: client}
}

func (c *SMSChannel) Type() string { return "sms" }

func (c *SMSChannel) SendBatched(
	ctx context.Context, msg Message, recipients []Recipient, config json.RawMessage,
) error {
	cfg, err := decodeSMSConfig(config)
	if err != nil {
		return err
	}
	if cfg.ProviderURL == "" {
		return fmt.Errorf("sms: provider_url is required")
	}
	if len(recipients) == 0 {
		return nil
	}

	var firstErr error
	for _, r := range recipients {
		number := strings.TrimSpace(r.Endpoint)
		if number == "" {
			continue
		}
		if err := c.sendOnce(ctx, cfg, number, msg); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *SMSChannel) sendOnce(ctx context.Context, cfg SMSConfig, number string, msg Message) error {
	envelope := map[string]any{
		"to":            number,
		"sign_name":     cfg.SignName,
		"template_code": cfg.TemplateCode,
		"incident_id":   msg.IncidentID,
		"severity":      msg.Severity,
		"title":         msg.Title,
		"body":          smsTrim(msg.Body),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ProviderURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.AuthHeader != "" {
		req.Header.Set("Authorization", cfg.AuthHeader)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sms: provider returned status %d for %s", resp.StatusCode, number)
	}
	return nil
}

// smsTrim caps the body at 280 chars (two SMS segments worth) so the
// provider doesn't reject the call for over-length content.  Most
// useful information is already in the title; the body acts as the
// "tail" of the alert.
func smsTrim(body string) string {
	const maxBytes = 280
	body = strings.TrimSpace(body)
	if len(body) <= maxBytes {
		return body
	}
	cut := maxBytes
	for cut > 0 && (body[cut]&0xC0) == 0x80 {
		cut--
	}
	return body[:cut] + "…"
}

func decodeSMSConfig(raw json.RawMessage) (SMSConfig, error) {
	var cfg SMSConfig
	if len(raw) == 0 {
		return cfg, fmt.Errorf("sms: missing provider config")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("sms: invalid config: %w", err)
	}
	return cfg, nil
}
