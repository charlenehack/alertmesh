package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FeishuConfig is unmarshalled from notification_contacts.config.
//
//	{
//	  "webhook_url": "https://open.feishu.cn/open-apis/bot/v2/hook/xxx",
//	  "secret":      "optional-signing-secret"
//	}
type FeishuConfig struct {
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`
}

type FeishuChannel struct{}

func (c *FeishuChannel) Type() string { return "feishu" }

// SendBatched posts a single Interactive Card to the configured webhook.
// Each recipient with a non-empty Mention contributes a `<at user_id=…>`
// span at the head of the body section so the Feishu client highlights
// and pushes the message to those users.  Recipients without a Mention
// are silently included in the audience by virtue of being in the
// channel; their Name is surfaced in the action footer for traceability.
func (c *FeishuChannel) SendBatched(
	ctx context.Context, msg Message, recipients []Recipient, config json.RawMessage,
) error {
	var cfg FeishuConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("feishu: invalid config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("feishu: webhook_url is required")
	}

	payload, err := buildFeishuCard(msg, cfg, recipients)
	if err != nil {
		return err
	}

	return postJSON(ctx, cfg.WebhookURL, payload)
}

// buildFeishuCard constructs the Interactive Card payload.
func buildFeishuCard(msg Message, cfg FeishuConfig, recipients []Recipient) (map[string]any, error) {
	headerColor := severityToFeishuColor(msg.Severity)

	body := msg.Body
	if mention := buildFeishuMention(recipients); mention != "" {
		body = mention + "\n\n" + body
	}

	// The card header already encodes severity (template = severity-derived
	// colour, title carries the "[Px]" tag), and msg.Body no longer
	// duplicates the alert severity, so we only need a single short field
	// for the incident id.  Keeps the card visually compact.
	elements := []any{
		map[string]any{
			"tag":    "div",
			"fields": []any{feishuShortField("**事件 ID**\n" + msg.IncidentID)},
		},
		map[string]any{
			"tag":  "div",
			"text": map[string]any{"tag": "lark_md", "content": body},
		},
	}

	if msg.URL != "" {
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []any{
				map[string]any{
					"tag":  "button",
					"text": map[string]any{"tag": "plain_text", "content": "查看事件"},
					"url":  msg.URL,
					"type": "primary",
				},
			},
		})
	}

	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"title":    map[string]any{"tag": "plain_text", "content": msg.Title},
			"template": headerColor,
		},
		"elements": elements,
	}

	payload := map[string]any{
		"msg_type": "interactive",
		"card":     card,
	}

	// Add HMAC-SHA256 signature when secret is configured.
	if cfg.Secret != "" {
		ts := time.Now().Unix()
		sign, err := feishuSign(ts, cfg.Secret)
		if err != nil {
			return nil, fmt.Errorf("feishu: sign error: %w", err)
		}
		payload["timestamp"] = fmt.Sprintf("%d", ts)
		payload["sign"] = sign
	}

	return payload, nil
}

// feishuSign computes the Feishu robot webhook signature.
// Formula: base64( HMAC-SHA256( key=secret, msg="{timestamp}\n{secret}" ) )
func feishuSign(timestamp int64, secret string) (string, error) {
	msg := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(msg)); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// buildFeishuMention concatenates Feishu @-mention spans for every
// recipient that carries an open_id / user_id in Mention.  Empty when
// no recipient is mentionable.
func buildFeishuMention(recipients []Recipient) string {
	if len(recipients) == 0 {
		return ""
	}
	var sb strings.Builder
	seen := make(map[string]struct{}, len(recipients))
	for _, r := range recipients {
		m := strings.TrimSpace(r.Mention)
		if m == "" {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(`<at user_id="`)
		sb.WriteString(m)
		sb.WriteString(`"></at>`)
	}
	return sb.String()
}

func feishuShortField(content string) map[string]any {
	return map[string]any{
		"is_short": true,
		"text":     map[string]any{"tag": "lark_md", "content": content},
	}
}

func severityToFeishuColor(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "red"
	case "warning":
		return "orange"
	case "resolved":
		return "green"
	default:
		return "blue"
	}
}

// postJSON sends a JSON payload via HTTP POST and checks for a non-2xx status.
func postJSON(ctx context.Context, url string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
