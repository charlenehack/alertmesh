package model

import (
	"time"

	"gorm.io/datatypes"
)

// SilencePolicy suppresses matching alerts for a defined time window.
type SilencePolicy struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name      string         `gorm:"not null"                                       json:"name"`
	Comment   string         `json:"comment"`
	Matchers  datatypes.JSON `gorm:"type:jsonb;not null"                            json:"matchers"` // [{key,op,value}]
	StartsAt  time.Time      `gorm:"not null"                                       json:"starts_at"`
	EndsAt    time.Time      `gorm:"not null"                                       json:"ends_at"`
	CreatedBy string         `gorm:"not null"                                       json:"created_by"`
	IsActive  bool           `gorm:"default:true;not null"                          json:"is_active"`

	Timestamps
}

func (SilencePolicy) TableName() string { return "silence_policies" }

// NotificationTemplate stores channel-specific message templates.
type NotificationTemplate struct {
	ID          string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string `gorm:"not null;uniqueIndex"                           json:"name"`
	ChannelType string `gorm:"not null"                                       json:"channel_type"` // dingtalk/feishu/slack/email/webhook
	Subject     string `json:"subject"`                                                            // for email
	Body        string `gorm:"type:text;not null"                             json:"body"`         // markdown / template string
	IsDefault   bool   `gorm:"default:false"                                  json:"is_default"`
	Description string `json:"description"`

	Timestamps
}

func (NotificationTemplate) TableName() string { return "notification_templates" }

// AggregationPolicy defines how alerts are grouped before becoming an incident.
type AggregationPolicy struct {
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name           string         `gorm:"not null"                                       json:"name"`
	Matchers       datatypes.JSON `gorm:"type:jsonb;not null"                            json:"matchers"`        // which alerts this policy applies to
	GroupBy        datatypes.JSON `gorm:"type:jsonb"                                     json:"group_by"`        // label keys to group by
	GroupWait      int            `gorm:"not null;default:30"                            json:"group_wait"`      // seconds
	GroupInterval  int            `gorm:"not null;default:300"                           json:"group_interval"`  // seconds
	RepeatInterval int            `gorm:"not null;default:3600"                          json:"repeat_interval"` // seconds
	IsEnabled      bool           `gorm:"default:true;not null"                          json:"is_enabled"`
	Description    string         `json:"description"`

	Timestamps
}

func (AggregationPolicy) TableName() string { return "aggregation_policies" }

// InhibitRule lets a "source" alert suppress matching "target" alerts while
// active.  The optional Equal list constrains suppression to alerts that share
// values on the listed labels (Alertmanager-style semantics).
type InhibitRule struct {
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name           string         `gorm:"not null"                                       json:"name"`
	SourceMatchers datatypes.JSON `gorm:"type:jsonb;not null"                            json:"source_matchers"`
	TargetMatchers datatypes.JSON `gorm:"type:jsonb;not null"                            json:"target_matchers"`
	Equal          datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"equal"`
	IsEnabled      bool           `gorm:"default:true;not null"                          json:"is_enabled"`
	Description    string         `json:"description"`

	Timestamps
}

func (InhibitRule) TableName() string { return "inhibit_rules" }

// EscalationPolicy bumps an open, unacked incident's severity once AckTimeout
// (in seconds) elapses without acknowledgement.
type EscalationPolicy struct {
	ID           string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name         string `gorm:"not null"                                       json:"name"`
	FromSeverity string `gorm:"not null"                                       json:"from_severity"`
	ToSeverity   string `gorm:"not null"                                       json:"to_severity"`
	AckTimeout   int    `gorm:"not null"                                       json:"ack_timeout"`
	IsEnabled    bool   `gorm:"default:true;not null"                          json:"is_enabled"`
	Description  string `json:"description"`

	Timestamps
}

func (EscalationPolicy) TableName() string { return "escalation_policies" }
