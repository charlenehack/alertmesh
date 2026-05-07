package model

import (
	"time"

	"gorm.io/datatypes"
)

// WebhookSource describes one trusted external alert source that posts to
// /api/v1/alerts/webhook/{source} using HTTP Message Signatures (RFC 9421)
// signed with a per-source Ed25519 keypair.
//
// The PRIVATE key is generated server-side and returned to the user only
// once on create / rotate; only the PEM-encoded PKIX public key is stored.
type WebhookSource struct {
	ID          string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string `gorm:"uniqueIndex;not null"                           json:"name"`       // == path {source}
	ClientID    string `gorm:"uniqueIndex;not null;type:varchar(64)"          json:"client_id"`  // RFC 9421 keyid
	PublicKey   string `gorm:"type:text;not null"                             json:"public_key"` // PEM Ed25519 PKIX
	AllowSkew   int    `gorm:"not null;default:300"                           json:"allow_skew"` // created ±N seconds
	IsEnabled   bool   `gorm:"not null;default:true"                          json:"is_enabled"`
	Description string `json:"description"`

	// Mapping is JSON matching ingestion.WebhookMapping (gjson paths into webhook body).
	Mapping datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"mapping,omitempty"`

	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	Timestamps
}

func (WebhookSource) TableName() string { return "webhook_sources" }
