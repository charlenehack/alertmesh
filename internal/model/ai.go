package model

import (
	"time"
)

type AITask struct {
	ID         string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	IncidentID string     `gorm:"not null;index"                                 json:"incident_id"`
	Status     string     `gorm:"not null;default:'pending'"                     json:"status"` // pending/running/done/failed
	Priority   int        `gorm:"default:0"                                      json:"priority"`
	Error      string     `json:"error,omitempty"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`

	Timestamps
}

func (AITask) TableName() string { return "ai_tasks" }

type AIAnalysis struct {
	ID         string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	IncidentID string `gorm:"not null;index"                                 json:"incident_id"`
	TaskID     string `gorm:"type:uuid;index"                                json:"task_id"`
	Report     string `gorm:"type:text;not null"                             json:"report"` // Markdown
	Summary    string `gorm:"type:text"                                      json:"summary"`
	RootCause  string `gorm:"type:text"                                      json:"root_cause"`

	Timestamps
}

func (AIAnalysis) TableName() string { return "ai_analyses" }

type AIConversation struct {
	ID         string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	IncidentID string `gorm:"not null;index"                                 json:"incident_id"`
	Role       string `gorm:"not null"                                       json:"role"` // user/assistant
	Content    string `gorm:"type:text;not null"                             json:"content"`
	UserID     string `gorm:"index"                                          json:"user_id"`

	Timestamps
}

func (AIConversation) TableName() string { return "ai_conversations" }
