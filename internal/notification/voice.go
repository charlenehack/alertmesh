package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// VoiceConfig is unmarshalled from a notification_contacts.config payload
// or — more commonly — synthesised by the Dispatcher from a system-wide
// `notification.voice` SystemConfig row.
//
// Two delivery modes are supported via a single generic POST envelope:
//
//  1. Provider gateway (Twilio / Aliyun-VMS / etc.):
//     The Dispatcher posts {"to":"<e164>", "title":..., "body":..., "incident_id":...}
//     to ProviderURL with the Authorization header set to AuthHeader.
//     The provider is responsible for synthesising a TTS announcement
//     and placing the call.
//
//  2. Plain webhook (on-prem PBX with custom integration):
//     Same envelope, no Authorization header.
//
// Voice is the only channel where SendBatched cannot collapse multiple
// recipients into a single API call — placing N calls sequentially is
// the natural lower bound — so the implementation iterates the
// Recipients slice and aggregates per-call errors.
type VoiceConfig struct {
	ProviderURL string `json:"provider_url"`
	AuthHeader  string `json:"auth_header"`
	// CallerID is rendered into the payload so the provider can present
	// a stable from-number to the receiver.  Empty leaves it to the
	// provider's default.
	CallerID string `json:"caller_id"`
}

type VoiceChannel struct {
	httpClient *http.Client
}

// NewVoiceChannel allows tests to inject a stub HTTP client.  Pass nil
// for production callers — http.DefaultClient is then used.
func NewVoiceChannel(client *http.Client) *VoiceChannel {
	if client == nil {
		client = http.DefaultClient
	}
	return &VoiceChannel{httpClient: client}
}

func (c *VoiceChannel) Type() string { return "voice" }

func (c *VoiceChannel) SendBatched(
	ctx context.Context, msg Message, recipients []Recipient, config json.RawMessage,
) error {
	cfg, err := decodeVoiceConfig(config)
	if err != nil {
		return err
	}
	if cfg.ProviderURL == "" {
		return fmt.Errorf("voice: provider_url is required")
	}
	if len(recipients) == 0 {
		return nil
	}

	var firstErr error
	for _, r := range recipients {
		number := strings.TrimSpace(r.Endpoint)
		if number == "" {
			// No phone — skip silently; the dispatcher's resolveRecipients
			// step is the right place to alert the operator about contacts
			// without a phone for a P0 policy.
			continue
		}
		if err := c.callOnce(ctx, cfg, number, msg); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *VoiceChannel) callOnce(ctx context.Context, cfg VoiceConfig, number string, msg Message) error {
	envelope := map[string]any{
		"to":          number,
		"caller_id":   cfg.CallerID,
		"incident_id": msg.IncidentID,
		"severity":    msg.Severity,
		"title":       msg.Title,
		"body":        msg.Body,
		"url":         msg.URL,
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
		return fmt.Errorf("voice: provider returned status %d for %s", resp.StatusCode, number)
	}
	return nil
}

func decodeVoiceConfig(raw json.RawMessage) (VoiceConfig, error) {
	var cfg VoiceConfig
	if len(raw) == 0 {
		return cfg, fmt.Errorf("voice: missing provider config")
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("voice: invalid config: %w", err)
	}
	return cfg, nil
}
