package notification

import (
	"context"
	"encoding/json"
)

// Recipient is the per-(contact × channel) target of a single dispatch.
//
// One contact may produce several Recipients across different channels —
// e.g. a DingTalk Recipient and an Email Recipient — but always exactly
// one Recipient per (contact, channel) pair.  The Dispatcher groups
// Recipients into per-channel buckets keyed by the shared address (e.g.
// the same DingTalk webhook URL) so a single API call can fan out to N
// people via the channel's native mechanism (@mention list, To/Cc list).
type Recipient struct {
	// ContactID is the source notification_contacts row.  Used by the
	// notification_log writer to record exactly who got notified.
	ContactID string

	// Name is the contact's display name — surfaced in log lines and as
	// the salutation in some channels (Email "Hi <Name>,", etc.).
	Name string

	// Endpoint is the channel-specific address for this recipient.
	// Interpretation:
	//   email     → the rfc5322 email address (e.g. "sre@x.com")
	//   voice/sms → the e.164 mobile number (e.g. "+8613800001111")
	//   dingtalk  → the contact's mobile, used for the atMobiles array
	//   feishu    → the contact's open_id / user_id; falls back to mobile
	//   slack     → the user/member id (Slack <@U123>); empty for plain
	//               webhook channels where there is no per-recipient @
	// Empty string means "no per-recipient address" — typical for IM
	// channels where the webhook itself defines the audience and only
	// the @mention varies (which is then captured by Mention).
	Endpoint string

	// Mention is the channel-native @-syntax for pinging this recipient
	// inside a shared-room IM message.  Examples:
	//   dingtalk → mobile (atMobiles entry)
	//   feishu   → "<at user_id=\"ou_xxx\"></at>"
	//   slack    → "<@U123>"
	// Empty string means "do not @-ping; just include in the message
	// body so the room sees the alert without a phone push".
	Mention string
}

// Channel is the interface every notification sender implements.
//
// SendBatched receives a list of Recipients that have already been
// grouped by the Dispatcher according to the channel's batching key (see
// dispatcher.groupByChannelTarget).  Implementations MUST issue one
// natural-batch API call per invocation — for IM that means a single
// webhook POST whose payload @-mentions every Recipient; for Email that
// means a single smtp.SendMail with all addresses in To/Cc; for
// voice/sms (which have no native batching) it means iterating the
// recipients sequentially inside SendBatched and bubbling up any
// per-call error.
//
// `cfg` is the raw JSON config shared by everyone in the bucket — the
// Dispatcher guarantees all Recipients in a single SendBatched call
// resolve to the same cfg (same webhook, same SMTP credentials, etc.),
// so implementations can unmarshal it once.
type Channel interface {
	Type() string
	SendBatched(ctx context.Context, msg Message, recipients []Recipient, cfg json.RawMessage) error
}
