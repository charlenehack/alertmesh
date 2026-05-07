package model

import (
	"time"

	"gorm.io/datatypes"
)

type Incident struct {
	ID       string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Title    string         `gorm:"not null"                                       json:"title"`
	Severity string         `gorm:"not null;index"                                 json:"severity"` // P0/P1/P2/P3
	Status   string         `gorm:"not null;index;default:'open'"                  json:"status"`   // open/ack/in_progress/resolved/closed
	Source   string         `gorm:"not null"                                       json:"source"`
	Labels   datatypes.JSON `gorm:"type:jsonb"                                     json:"labels"`
	GroupKey string         `gorm:"index"                                          json:"group_key"`
	RouteID  *string        `gorm:"type:uuid;index"                                json:"route_id,omitempty"`
	// DataSourceID is the data_sources.id this incident's first alert came
	// from; threaded all the way from the ingestion layer.  Used by the AI
	// gate to look up ai_enabled / ai_auto_trigger and by router.ai.trigger.
	// NULL for legacy incidents
	// created before migration 000038, in which case AI is treated as
	// disabled.
	DataSourceID *string    `gorm:"type:uuid;index"                                json:"data_source_id,omitempty"`
	AssigneeID   *string    `json:"assignee_id"`
	OpenedAt     time.Time  `gorm:"autoCreateTime"                                 json:"opened_at"`
	AckedAt      *time.Time `json:"acked_at"`
	ResolvedAt   *time.Time `json:"resolved_at"`
	// LastAlertAt is bumped to now() every time a new firing alert lands in
	// this incident (createIncident, appendToIncident, reopenIncident).
	// Used by the staleness reaper to decide which open incidents have gone
	// silent for longer than incident.staleness_timeout and should be
	// auto-resolved.  Backfilled to opened_at by migration 000041.
	LastAlertAt *time.Time `gorm:"index"                                          json:"last_alert_at,omitempty"`
	// ParentIncidentID points at the previous incident this one continues
	// from when an alert with the same group_key fired again *after* the
	// reopen window had already closed (so we created a fresh row instead
	// of reopening).  Lets the UI render a "延续自 #xxx" link.  NULL when
	// the incident is brand-new for its group_key.
	ParentIncidentID *string `gorm:"type:uuid;index"                                json:"parent_incident_id,omitempty"`
	// AutoResolvedAt is set when the staleness reaper or a Prometheus
	// endsAt≤now signal closed the incident automatically (vs. an operator
	// clicking Resolve in the UI).  Allows the timeline / metrics to
	// distinguish manual vs. automatic closure.
	AutoResolvedAt *time.Time `json:"auto_resolved_at,omitempty"`
	// NotificationCount counts every successful outbound notification dispatch
	// (initial + repeats + escalation + reopened).  Surfaced in the UI so
	// the on-call can tell at a glance how noisy this incident has been.
	NotificationCount int `gorm:"not null;default:0"                             json:"notification_count"`
	// LastNotifiedAt is updated at the end of every dispatch round.  Used
	// by maybeRepeatNotify to gate the progressive repeat schedule —
	// replaces the previous MAX(notification_log.sent_at) aggregate query
	// with an O(1) column read.
	LastNotifiedAt *time.Time `json:"last_notified_at,omitempty"`
	// RepeatSeqIndex tracks how many re-notifications have already fired
	// at the current severity tier.  Drives the linear-increasing repeat
	// cadence (1m → 3m → 5m → 5m+step → … capped at interval_max) defined
	// in notification.repeat_schedule.  Reset to 0 on every severity
	// escalation so the new tier starts at the dense head of the sequence
	// again.  See incident/service.go: computeInterval / bumpSeqIndex.
	RepeatSeqIndex int `gorm:"not null;default:0"                             json:"repeat_seq_index"`
	// SeverityStartedAt is the anchor used by the dwell-based escalator —
	// when time.Since(severity_started_at) ≥ tier.dwell, the lifecycle
	// bumps severity one rung and stamps this column with now() so the
	// next dwell window starts fresh.  Set on createIncident and
	// reopenIncident; nil for legacy rows pre-migration 000045 (handled
	// transparently by falling back to opened_at).
	SeverityStartedAt *time.Time `json:"severity_started_at,omitempty"`
	AIStatus          string     `gorm:"default:'pending'"                              json:"ai_status"` // pending/running/done/failed
	AIReportID        *string    `gorm:"type:uuid"                                      json:"ai_report_id"`

	Alerts   []Alert            `gorm:"foreignKey:IncidentID" json:"alerts,omitempty"`
	Timeline []IncidentTimeline `gorm:"foreignKey:IncidentID" json:"timeline,omitempty"`

	Timestamps
}

func (Incident) TableName() string { return "incidents" }

// Status constants
const (
	IncidentStatusOpen       = "open"
	IncidentStatusAck        = "ack"
	IncidentStatusInProgress = "in_progress"
	IncidentStatusResolved   = "resolved"
	IncidentStatusClosed     = "closed"
)

// Severity constants
const (
	SeverityP0 = "P0"
	SeverityP1 = "P1"
	SeverityP2 = "P2"
	SeverityP3 = "P3"
)

// AI status constants.
//
// `disabled` is set on incidents whose owning data source either has
// ai_enabled=false or whose kind isn't on the LLM whitelist (kafka /
// opensearch / elastic).  It tells the frontend to hide the "触发 AI 分析" button
// and the AI tab entirely; /ai/trigger also rejects the call.
const (
	AIStatusPending  = "pending"
	AIStatusRunning  = "running"
	AIStatusDone     = "done"
	AIStatusFailed   = "failed"
	AIStatusDisabled = "disabled"
)
