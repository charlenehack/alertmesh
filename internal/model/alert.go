package model

import (
	"time"

	"gorm.io/datatypes"
)

type Alert struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	IncidentID  string         `gorm:"not null;index"                                 json:"incident_id"`
	Source      string         `gorm:"not null"                                       json:"source"`
	Fingerprint string         `gorm:"not null;index"                                 json:"fingerprint"`
	Labels      datatypes.JSON `gorm:"type:jsonb"                                     json:"labels"`
	Annotations datatypes.JSON `gorm:"type:jsonb"                                     json:"annotations"`
	StartsAt    time.Time      `json:"starts_at"`
	EndsAt      *time.Time     `json:"ends_at"`
	Status      string         `gorm:"not null"                                       json:"status"` // firing/resolved
	RawPayload  []byte         `gorm:"type:jsonb"                                     json:"raw_payload,omitempty"`

	Timestamps
}

func (Alert) TableName() string { return "alerts" }

const (
	AlertStatusFiring   = "firing"
	AlertStatusResolved = "resolved"
)
