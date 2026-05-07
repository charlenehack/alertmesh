package model

import "time"

type NotificationLog struct {
	ID          string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	IncidentID  string    `gorm:"not null;index"                                 json:"incident_id"`
	ChannelID   string    `gorm:"not null;index"                                 json:"channel_id"`
	ChannelType string    `gorm:"not null"                                       json:"channel_type"`
	Status      string    `gorm:"not null"                                       json:"status"` // sent/failed
	Error       string    `json:"error,omitempty"`
	SentAt      time.Time `gorm:"autoCreateTime"                                 json:"sent_at"`
}

func (NotificationLog) TableName() string { return "notification_log" }
