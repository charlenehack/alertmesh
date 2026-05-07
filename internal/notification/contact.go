package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	cfgcrypto "github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/pkg/metrics"
)

// resolvedRecipient is the internal product of resolveRecipients — one
// (channel, contact) pair plus the per-bucket key and raw config the
// dispatcher will hand to Channel.SendBatched.  Every channel a contact
// has populated produces exactly one resolvedRecipient.
type resolvedRecipient struct {
	ChannelType string
	BucketKey   string          // shared address (webhook URL / SMTP host / "voice")
	Config      json.RawMessage // unmarshalled by the channel impl in SendBatched
	Recipient   Recipient
}

// dispatchToContacts is the top-level fan-out called by DispatchForIncident.
// Three stages:
//
//  1. resolveRecipients — walk every contact × every applicable channel,
//     producing one resolvedRecipient per channel-the-contact-has-config-for.
//     Voice and SMS are HARDCODED to severity == "P0" only — the
//     NotificationPolicy severities filter is the right place to gate
//     IM/email by severity, but the on-call would be (rightly) furious
//     if a P3 woke them up by phone, so we enforce it once here.
//  2. groupByChannelTarget — bucket recipients by (channel, BucketKey).
//     Recipients sharing a bucket get merged into a single SendBatched
//     call; recipients in different buckets fan out as separate calls.
//  3. dispatchBuckets — actually invoke each Channel.SendBatched and
//     write per-contact notification_log rows + Prometheus counters.
func (d *Dispatcher) dispatchToContacts(
	ctx context.Context, msg Message, contacts []model.NotificationContact,
) {
	resolved := d.resolveRecipients(ctx, msg, contacts)
	if len(resolved) == 0 {
		log.Debug().
			Str("incident_id", msg.IncidentID).
			Str("severity", msg.Severity).
			Int("contacts", len(contacts)).
			Msg("dispatcher: zero recipients resolved (contacts have no usable channel config)")
		return
	}

	buckets := groupByChannelTarget(resolved)
	d.dispatchBuckets(ctx, msg, buckets)
}

// resolveRecipients enumerates every channel each contact has configured
// and returns one resolvedRecipient per (contact, channel) tuple.  It is
// the only place that knows the contact-row → channel-config translation
// (which webhook field maps to which channel, what counts as a usable
// config, etc.) — keeping it isolated here means adding a new channel
// is a single switch-arm change instead of a search across the
// dispatcher.
func (d *Dispatcher) resolveRecipients(
	ctx context.Context, msg Message, contacts []model.NotificationContact,
) []resolvedRecipient {
	out := make([]resolvedRecipient, 0, len(contacts)*2)

	// Lazily load shared SystemConfigs the first time a contact needs them
	// so the common case (no email contacts → no SMTP fetch) is free.
	var (
		smtpCfg     EmailConfig
		smtpLoaded  bool
		smtpOK      bool
		voiceRaw    json.RawMessage
		voiceLoaded bool
		smsRaw      json.RawMessage
		smsLoaded   bool
	)

	isP0 := strings.EqualFold(strings.TrimSpace(msg.Severity), "P0")

	for i := range contacts {
		c := &contacts[i]

		// DingTalk — bucket key = webhook URL + secret (different secrets
		// hash to different signed URLs at send time, so they MUST live
		// in different buckets).
		if v := strings.TrimSpace(c.DingtalkWebhook); v != "" {
			cfgRaw, _ := json.Marshal(DingTalkConfig{
				WebhookURL: v,
				Secret:     c.DingtalkSecret,
			})
			out = append(out, resolvedRecipient{
				ChannelType: "dingtalk",
				BucketKey:   "dingtalk::" + v + "::" + c.DingtalkSecret,
				Config:      cfgRaw,
				Recipient: Recipient{
					ContactID: c.ID,
					Name:      c.Name,
					Endpoint:  c.Phone, // not used by dingtalk impl
					Mention:   c.Phone, // atMobiles entry
				},
			})
		}

		// Feishu — same bucketing rule as DingTalk.
		if v := strings.TrimSpace(c.FeishuWebhook); v != "" {
			cfgRaw, _ := json.Marshal(FeishuConfig{
				WebhookURL: v,
				Secret:     c.FeishuSecret,
			})
			out = append(out, resolvedRecipient{
				ChannelType: "feishu",
				BucketKey:   "feishu::" + v + "::" + c.FeishuSecret,
				Config:      cfgRaw,
				Recipient: Recipient{
					ContactID: c.ID,
					Name:      c.Name,
					Endpoint:  c.Phone,
					// Phone happens to be the most common mention key
					// for Feishu robots that ship without an open_id —
					// operators with a real open_id should set it as
					// the contact's phone field for now (a dedicated
					// FeishuUserID field is on the v3.1 backlog).
					Mention: c.Phone,
				},
			})
		}

		// Slack — bot-token mode and webhook mode each get their own
		// bucket key; a contact configured both ways picks bot mode
		// (matching the legacy dispatchToContact precedence).
		switch {
		case c.SlackBotToken != "" && c.SlackChannelID != "":
			cfgRaw, _ := json.Marshal(SlackConfig{
				BotToken:  c.SlackBotToken,
				ChannelID: c.SlackChannelID,
			})
			out = append(out, resolvedRecipient{
				ChannelType: "slack",
				BucketKey:   "slack-bot::" + c.SlackBotToken + "::" + c.SlackChannelID,
				Config:      cfgRaw,
				Recipient: Recipient{
					ContactID: c.ID,
					Name:      c.Name,
					Mention:   c.Phone, // operators put the Slack user id here
				},
			})
		case c.WebhookURL != "":
			cfgRaw, _ := json.Marshal(SlackConfig{WebhookURL: c.WebhookURL})
			out = append(out, resolvedRecipient{
				ChannelType: "slack",
				BucketKey:   "slack-webhook::" + c.WebhookURL,
				Config:      cfgRaw,
				Recipient: Recipient{
					ContactID: c.ID,
					Name:      c.Name,
					Mention:   c.Phone,
				},
			})
		}

		// Email — single shared SMTP bucket.
		if e := strings.TrimSpace(c.Email); e != "" {
			if !smtpLoaded {
				smtpCfg, smtpOK = d.loadSMTPConfig(ctx)
				smtpLoaded = true
			}
			if smtpOK {
				// Marshal a per-call config WITHOUT the To list — the
				// Channel.SendBatched implementation appends every
				// recipient's Endpoint into To at send time.
				cfgRaw, _ := json.Marshal(smtpCfg)
				out = append(out, resolvedRecipient{
					ChannelType: "email",
					BucketKey:   "email::" + smtpCfg.SMTPHost + "::" + smtpCfg.From,
					Config:      cfgRaw,
					Recipient: Recipient{
						ContactID: c.ID,
						Name:      c.Name,
						Endpoint:  e,
					},
				})
			} else {
				log.Debug().Str("contact", c.Name).
					Msg("dispatcher: email skipped (no system smtp config)")
			}
		}

		// Voice & SMS — hardcoded P0-only gate per project policy.
		// Both channels read their provider config from system_configs
		// (notification.voice / notification.sms) so operators don't
		// have to duplicate provider credentials per contact.
		if isP0 && strings.TrimSpace(c.Phone) != "" {
			if !voiceLoaded {
				voiceRaw = d.loadProviderConfig(ctx, "notification.voice")
				voiceLoaded = true
			}
			if len(voiceRaw) > 0 {
				out = append(out, resolvedRecipient{
					ChannelType: "voice",
					BucketKey:   "voice",
					Config:      voiceRaw,
					Recipient: Recipient{
						ContactID: c.ID,
						Name:      c.Name,
						Endpoint:  c.Phone,
					},
				})
			} else {
				log.Warn().Str("contact", c.Name).
					Msg("dispatcher: P0 voice skipped (notification.voice SystemConfig missing)")
			}

			if !smsLoaded {
				smsRaw = d.loadProviderConfig(ctx, "notification.sms")
				smsLoaded = true
			}
			if len(smsRaw) > 0 {
				out = append(out, resolvedRecipient{
					ChannelType: "sms",
					BucketKey:   "sms",
					Config:      smsRaw,
					Recipient: Recipient{
						ContactID: c.ID,
						Name:      c.Name,
						Endpoint:  c.Phone,
					},
				})
			} else {
				log.Warn().Str("contact", c.Name).
					Msg("dispatcher: P0 sms skipped (notification.sms SystemConfig missing)")
			}
		}
	}
	return out
}

// channelBucket holds every recipient that shares the (channel, key)
// tuple — i.e. every contact whose channel config resolves to the same
// upstream API call (same DingTalk webhook, same SMTP server, …).
type channelBucket struct {
	ChannelType string
	BucketKey   string
	Config      json.RawMessage
	Recipients  []Recipient
	ContactIDs  []string // mirrors Recipients ContactID for the log writer
}

// groupByChannelTarget collapses []resolvedRecipient into one
// channelBucket per (ChannelType, BucketKey).  Order is preserved by
// first-seen so the resulting log rows match the contact list order.
func groupByChannelTarget(in []resolvedRecipient) []channelBucket {
	if len(in) == 0 {
		return nil
	}
	type bucketRef struct {
		Index int
	}
	idx := make(map[string]bucketRef, len(in))
	out := make([]channelBucket, 0, len(in))

	for _, r := range in {
		key := r.ChannelType + "|" + r.BucketKey
		if ref, ok := idx[key]; ok {
			out[ref.Index].Recipients = append(out[ref.Index].Recipients, r.Recipient)
			out[ref.Index].ContactIDs = append(out[ref.Index].ContactIDs, r.Recipient.ContactID)
			continue
		}
		idx[key] = bucketRef{Index: len(out)}
		out = append(out, channelBucket{
			ChannelType: r.ChannelType,
			BucketKey:   r.BucketKey,
			Config:      r.Config,
			Recipients:  []Recipient{r.Recipient},
			ContactIDs:  []string{r.Recipient.ContactID},
		})
	}
	return out
}

// dispatchBuckets calls Channel.SendBatched for each bucket, writes a
// notification_log row per contact in the bucket, and bumps the
// dispatched/dropped counters with the right batched= label.
//
// Per-bucket failures DO NOT abort the loop — every bucket is an
// independent target and a failed DingTalk webhook should not stop the
// SMTP path from delivering.
func (d *Dispatcher) dispatchBuckets(ctx context.Context, msg Message, buckets []channelBucket) {
	for _, b := range buckets {
		sender, ok := d.channels[b.ChannelType]
		if !ok {
			log.Warn().
				Str("channel", b.ChannelType).
				Str("incident_id", msg.IncidentID).
				Msg("dispatcher: unknown channel type, skipping bucket")
			continue
		}
		batched := len(b.Recipients) > 1
		batchedLabel := "false"
		if batched {
			batchedLabel = "true"
		}

		err := sender.SendBatched(ctx, msg, b.Recipients, b.Config)
		if err != nil {
			log.Error().Err(err).
				Str("channel", b.ChannelType).
				Str("incident_id", msg.IncidentID).
				Int("recipients", len(b.Recipients)).
				Msg("dispatcher: bucket send failed")
			for i, cid := range b.ContactIDs {
				d.logNotification(ctx, msg.IncidentID, cid, b.ChannelType, "failed", err.Error())
				_ = i
			}
			continue
		}
		log.Info().
			Str("channel", b.ChannelType).
			Str("incident_id", msg.IncidentID).
			Int("recipients", len(b.Recipients)).
			Bool("batched", batched).
			Msg("dispatcher: bucket sent")
		for _, cid := range b.ContactIDs {
			d.logNotification(ctx, msg.IncidentID, cid, b.ChannelType, "sent", "")
			metrics.NotificationsDispatched.WithLabelValues(b.ChannelType, batchedLabel).Inc()
		}
	}
}

// loadSMTPConfig fetches the system-wide SMTP settings (notification.smtp key).
// The value is a JSON-encoded EmailConfig (without the To field, which is set
// per contact).  Returns ok=false if no config is present or it can't be parsed.
func (d *Dispatcher) loadSMTPConfig(ctx context.Context) (EmailConfig, bool) {
	raw := d.loadProviderConfig(ctx, "notification.smtp")
	if len(raw) == 0 {
		return EmailConfig{}, false
	}
	var cfg EmailConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		log.Warn().Err(err).Msg("notification.smtp: invalid JSON")
		return EmailConfig{}, false
	}
	if cfg.SMTPHost == "" {
		return EmailConfig{}, false
	}
	return cfg, true
}

// loadProviderConfig is the shared SystemConfig loader for the three
// system-wide channel configs (notification.smtp / .voice / .sms).
// AES-decrypts the value when an encryption key is set; returns nil
// (not an error) when the key is missing — every caller treats nil as
// "channel disabled, skip silently".
func (d *Dispatcher) loadProviderConfig(ctx context.Context, key string) json.RawMessage {
	var row model.SystemConfig
	if err := d.db.WithContext(ctx).
		Where("key = ?", key).
		First(&row).Error; err != nil {
		return nil
	}
	value := strings.TrimSpace(row.Value)
	if value == "" {
		return nil
	}
	if d.encryptionKey != "" {
		if plain, err := cfgcrypto.Decrypt(value, d.encryptionKey); err == nil {
			value = plain
		}
	}
	return json.RawMessage(value)
}

// loadAndDecryptContacts collects the union of direct contact_ids and
// contact_ids expanded from contact_groups, deduplicates, and returns
// the contact rows with their secret fields decrypted in place.
func (d *Dispatcher) loadAndDecryptContacts(
	ctx context.Context,
	contactIDs, groupIDs []string,
	encryptionKey string,
) []model.NotificationContact {
	idSet := make(map[string]struct{})
	for _, id := range contactIDs {
		if id != "" {
			idSet[id] = struct{}{}
		}
	}

	if len(groupIDs) > 0 {
		var groups []model.NotificationContactGroup
		if err := d.db.WithContext(ctx).
			Where("id IN ?", groupIDs).
			Find(&groups).Error; err == nil {
			for _, g := range groups {
				var members []string
				if len(g.ContactIDs) > 0 {
					_ = json.Unmarshal(g.ContactIDs, &members)
				}
				for _, id := range members {
					if id != "" {
						idSet[id] = struct{}{}
					}
				}
			}
		}
	}

	if len(idSet) == 0 {
		return nil
	}

	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	var contacts []model.NotificationContact
	if err := d.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&contacts).Error; err != nil {
		log.Warn().Err(err).Msg("dispatcher: load contacts failed")
		return nil
	}

	for i := range contacts {
		decryptContactInPlace(&contacts[i], encryptionKey)
	}
	return contacts
}

func decryptContactInPlace(c *model.NotificationContact, key string) {
	if key == "" {
		return
	}
	dec := func(v string) string {
		if v == "" {
			return ""
		}
		if plain, err := cfgcrypto.Decrypt(v, key); err == nil {
			return plain
		}
		return v
	}
	c.WebhookToken = dec(c.WebhookToken)
	c.SlackBotToken = dec(c.SlackBotToken)
	c.FeishuSecret = dec(c.FeishuSecret)
	c.DingtalkSecret = dec(c.DingtalkSecret)
}

// severityMatches returns true when sev matches at least one entry in the
// JSON-encoded severities array.  An empty array means "all severities".
func severityMatches(severities []byte, sev string) bool {
	if len(severities) == 0 {
		return true
	}
	var list []string
	if err := json.Unmarshal(severities, &list); err != nil {
		return true
	}
	if len(list) == 0 {
		return true
	}
	for _, s := range list {
		if s == sev {
			return true
		}
	}
	return false
}

// channelIDsToPolicyIDs decodes route.ChannelIDs (a JSON array of policy UUIDs).
// Empty input → nil.  Invalid JSON → returns nil with a warning.
func channelIDsToPolicyIDs(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		log.Warn().Err(err).Msg("alert_route.channel_ids: invalid JSON, expected []string")
		return nil
	}
	return ids
}

// joinPolicyIDs is a small debug helper.
func joinPolicyIDs(p []model.NotificationPolicy) string {
	if len(p) == 0 {
		return ""
	}
	out := ""
	for i, x := range p {
		if i > 0 {
			out += ","
		}
		out += x.Name
	}
	return fmt.Sprintf("[%s]", out)
}

// gormErrorIsNoRow is a tiny helper to make read code cleaner.
func gormErrorIsNoRow(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
