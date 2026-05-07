package model

import (
	"gorm.io/datatypes"
)

// NotificationPolicy is the user-facing "通知策略".
type NotificationPolicy struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"not null;uniqueIndex"                           json:"name"`
	Severities  datatypes.JSON `gorm:"type:jsonb"                                     json:"severities"`
	Description string         `json:"description"`
	ContactIDs  datatypes.JSON `gorm:"type:jsonb"                                     json:"contact_ids"`
	GroupIDs    datatypes.JSON `gorm:"type:jsonb"                                     json:"group_ids"`
	IsEnabled   bool           `gorm:"default:true;not null"                          json:"is_enabled"`

	Timestamps
}

func (NotificationPolicy) TableName() string { return "notification_policies" }

// NotificationContact is a single recipient (联系人) with multiple delivery endpoints.
// Fields tagged "secret" are AES-encrypted at rest.
type NotificationContact struct {
	ID   string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name string `gorm:"not null;uniqueIndex"                           json:"name"`

	Email string `gorm:"type:varchar(255)" json:"email"`
	Phone string `gorm:"type:varchar(32)"  json:"phone"`

	// Webhook – generic
	WebhookURL   string `gorm:"type:text" json:"webhook_url"`
	WebhookToken string `gorm:"type:text" json:"webhook_token"` // encrypted

	// Slack – Bot Token + Channel
	SlackBotToken  string `gorm:"type:text" json:"slack_bot_token"` // encrypted
	SlackChannelID string `gorm:"type:text" json:"slack_channel_id"`

	// Feishu – Webhook + Secret
	FeishuWebhook string `gorm:"type:text" json:"feishu_webhook"`
	FeishuSecret  string `gorm:"type:text" json:"feishu_secret"` // encrypted

	// DingTalk – Webhook + Secret
	DingtalkWebhook string `gorm:"type:text" json:"dingtalk_webhook"`
	DingtalkSecret  string `gorm:"type:text" json:"dingtalk_secret"` // encrypted

	Timestamps
}

func (NotificationContact) TableName() string { return "notification_contacts" }

// SecretFields returns the field names that must be encrypted at rest.
func (NotificationContact) SecretFields() []string {
	return []string{"WebhookToken", "SlackBotToken", "FeishuSecret", "DingtalkSecret"}
}

// NotificationContactGroup is a named set of contacts (联系人组).
type NotificationContactGroup struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"not null;uniqueIndex"                           json:"name"`
	Description string         `json:"description"`
	ContactIDs  datatypes.JSON `gorm:"type:jsonb"                                     json:"contact_ids"`

	Timestamps
}

func (NotificationContactGroup) TableName() string { return "notification_contact_groups" }
