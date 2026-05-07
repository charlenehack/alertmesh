package model

type IncidentTimeline struct {
	ID         string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	IncidentID string `gorm:"not null;index"                                 json:"incident_id"`
	Action     string `gorm:"not null"                                       json:"action"` // created/acked/assigned/commented/resolved/closed/escalated/ai_triggered
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	UserID     string `gorm:"index"                                          json:"user_id"`
	Username   string `json:"username"`
	Message    string `gorm:"type:text"                                      json:"message"`

	Timestamps
}

func (IncidentTimeline) TableName() string { return "incident_timeline" }
