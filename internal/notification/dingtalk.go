package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// DingTalkConfig is unmarshalled from notification_contacts.config.
//
//	{
//	  "webhook_url": "https://oapi.dingtalk.com/robot/send?access_token=xxx",
//	  "secret":      "optional-signing-secret"
//	}
type DingTalkConfig struct {
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`
}

type DingTalkChannel struct{}

func (c *DingTalkChannel) Type() string { return "dingtalk" }

// SendBatched posts ONE message to the configured webhook that @-mentions
// every recipient in the batch via the atMobiles array.  Recipients that
// have no Mention (empty mobile) are still listed in the message body so
// the room operator can see who the alert was meant for, but won't get a
// per-device push.
func (c *DingTalkChannel) SendBatched(
	ctx context.Context, msg Message, recipients []Recipient, config json.RawMessage,
) error {
	var cfg DingTalkConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("dingtalk: invalid config: %w", err)
	}
	if cfg.WebhookURL == "" {
		return fmt.Errorf("dingtalk: webhook_url is required")
	}

	webhookURL := cfg.WebhookURL
	if cfg.Secret != "" {
		ts := time.Now().UnixMilli()
		sign := dingtalkSign(ts, cfg.Secret)
		webhookURL += fmt.Sprintf("&timestamp=%d&sign=%s", ts, url.QueryEscape(sign))
	}

	payload := buildDingTalkPayload(msg, recipients)
	return postJSON(ctx, webhookURL, payload)
}

// buildDingTalkPayload renders the markdown body and the at.atMobiles
// list.  When recipients has ≥1 entry with a Mention, the trailing line
// of the body explicitly @-pings each so the DingTalk client surfaces a
// device push (atMobiles alone does not render an @ in markdown
// messages, the @<mobile> token in the body is what triggers the
// highlight).
//
// We deliberately do NOT prepend a "**告警级别:**" blockquote here:
// msg.Title already carries the severity tag (e.g. "[P3] route-name"),
// and buildNotificationBody no longer emits that line in msg.Body —
// printing it a third time in the adapter just adds noise.
func buildDingTalkPayload(msg Message, recipients []Recipient) map[string]any {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s\n\n", msg.Title))
	sb.WriteString(fmt.Sprintf("> **事件 ID:** %s\n\n", msg.IncidentID))
	sb.WriteString(msg.Body + "\n\n")
	if msg.URL != "" {
		sb.WriteString(fmt.Sprintf("[查看事件](%s)\n\n", msg.URL))
	}

	mobiles := make([]string, 0, len(recipients))
	seen := make(map[string]struct{}, len(recipients))
	var ats strings.Builder
	for _, r := range recipients {
		m := strings.TrimSpace(r.Mention)
		if m == "" {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		mobiles = append(mobiles, m)
		ats.WriteString("@")
		ats.WriteString(m)
		ats.WriteString(" ")
	}
	if ats.Len() > 0 {
		sb.WriteString(strings.TrimSpace(ats.String()))
	}

	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]any{
			"title": msg.Title,
			"text":  sb.String(),
		},
	}
	if len(mobiles) > 0 {
		payload["at"] = map[string]any{
			"atMobiles": mobiles,
			"isAtAll":   false,
		}
	}
	return payload
}

// dingtalkSign computes the DingTalk robot webhook signature.
// Formula: base64( HMAC-SHA256( key=secret, msg="{timestamp}\n{secret}" ) )
func dingtalkSign(timestamp int64, secret string) string {
	msg := fmt.Sprintf("%d\n%s", timestamp, secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

