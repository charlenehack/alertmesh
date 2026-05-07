package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SlackConfig is unmarshalled from notification_contacts.config.
//
// Two delivery modes are supported:
//
//  1. Incoming Webhook (simplest, no OAuth):
//     { "webhook_url": "https://hooks.slack.com/services/T.../B.../xxx" }
//
//  2. Slack App / Bot Token (supports multiple channels, richer features):
//     { "bot_token": "xoxb-...", "channel_id": "C0123456789" }
//
// If both are provided, Bot Token mode takes priority.
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	BotToken   string `json:"bot_token"`
	ChannelID  string `json:"channel_id"`
}

const slackAPIPostMessage = "https://slack.com/api/chat.postMessage"

type SlackChannel struct{}

func (c *SlackChannel) Type() string { return "slack" }

// SendBatched issues a single chat.postMessage (bot-token mode) or
// webhook POST (incoming-webhook mode) and prepends one `<@Uxxx>`
// mention per recipient that carries a Slack member id in Mention.
// Webhooks tied to a single channel inherently fan out to all members,
// so the @-mentions are purely a "ping these people specifically"
// signal layered on top of the room broadcast.
func (c *SlackChannel) SendBatched(
	ctx context.Context, msg Message, recipients []Recipient, config json.RawMessage,
) error {
	var cfg SlackConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("slack: invalid config: %w", err)
	}

	payload := buildSlackPayload(msg, recipients)

	switch {
	case cfg.BotToken != "" && cfg.ChannelID != "":
		return sendSlackBotToken(ctx, cfg.BotToken, cfg.ChannelID, payload)
	case cfg.WebhookURL != "":
		return sendSlackWebhook(ctx, cfg.WebhookURL, payload)
	default:
		return fmt.Errorf("slack: supply either bot_token+channel_id or webhook_url")
	}
}

// sendSlackWebhook posts to an Incoming Webhook URL.
// Slack returns plain text "ok" on success.
func sendSlackWebhook(ctx context.Context, webhookURL string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
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
		return fmt.Errorf("slack webhook: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// sendSlackBotToken calls chat.postMessage with a Bot User OAuth Token.
// Slack always returns HTTP 200; errors are reported in the JSON body.
func sendSlackBotToken(ctx context.Context, token, channelID string, payload map[string]any) error {
	// chat.postMessage requires "channel" in the body.
	payload["channel"] = channelID

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, slackAPIPostMessage, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("slack api: failed to decode response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("slack api: %s", result.Error)
	}
	return nil
}

// buildSlackPayload constructs a coloured attachment with Block Kit content.
// The top-level "text" field acts as notification fallback for clients that
// don't render Block Kit (e.g. mobile push notifications).
func buildSlackPayload(msg Message, recipients []Recipient) map[string]any {
	color := severityToSlackColor(msg.Severity)

	body := msg.Body
	if mention := buildSlackMention(recipients); mention != "" {
		body = mention + "\n" + body
	}

	// Severity already shows up in the header text (msg.Title carries
	// the "[Px]" prefix) AND in the attachment colour bar, so the
	// previous "*告警级别*" field block here was the third copy on
	// screen.  Drop it; keep only the incident id.
	blocks := []any{
		map[string]any{
			"type": "header",
			"text": map[string]any{"type": "plain_text", "text": msg.Title, "emoji": true},
		},
		map[string]any{
			"type": "section",
			"fields": []any{
				map[string]any{"type": "mrkdwn", "text": "*事件 ID*\n`" + msg.IncidentID + "`"},
			},
		},
		map[string]any{
			"type": "section",
			"text": map[string]any{"type": "mrkdwn", "text": body},
		},
	}

	if msg.URL != "" {
		blocks = append(blocks, map[string]any{
			"type": "actions",
			"elements": []any{
				map[string]any{
					"type":  "button",
					"style": "primary",
					"text":  map[string]any{"type": "plain_text", "text": "查看事件"},
					"url":   msg.URL,
				},
			},
		})
	}

	blocks = append(blocks, map[string]any{"type": "divider"})

	return map[string]any{
		"text": msg.Title, // fallback / push notification text
		"attachments": []any{
			map[string]any{
				"color":  color,
				"blocks": blocks,
			},
		},
	}
}

// buildSlackMention joins every recipient's Slack member id into a
// space-separated `<@U123>` block.  Empty when no recipient carries a
// usable Mention.
func buildSlackMention(recipients []Recipient) string {
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
		sb.WriteString("<@")
		sb.WriteString(m)
		sb.WriteString(">")
	}
	return sb.String()
}

func severityToSlackColor(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "#FF0000"
	case "warning":
		return "#FFA500"
	case "resolved":
		return "#36A64F"
	default:
		return "#439FE0"
	}
}
