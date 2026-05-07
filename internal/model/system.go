package model

import (
	"time"

	"gorm.io/datatypes"
)

// SystemConfig is a simple key/value store for system-level configuration.
// It intentionally has no timestamp columns – rows are static settings that
// are either bootstrapped at startup or updated through the admin API.
type SystemConfig struct {
	Key         string `gorm:"primaryKey;type:varchar(255)" json:"key"`
	Value       string `gorm:"type:text;not null"           json:"value"`
	Description string `gorm:"type:text"                    json:"description,omitempty"`
}

func (SystemConfig) TableName() string { return "system_configs" }

type AuditLog struct {
	ID       string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID   string         `gorm:"index"                                          json:"user_id"`
	Username string         `json:"username"`
	Action   string         `gorm:"not null"                                       json:"action"`
	Resource string         `gorm:"not null"                                       json:"resource"`
	Detail   datatypes.JSON `gorm:"type:jsonb"                                     json:"detail"`
	IP       string         `json:"ip"`

	Timestamps
}

func (AuditLog) TableName() string { return "audit_logs" }

type OncallSchedule struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"not null;index"                                 json:"user_id"`
	StartTime time.Time `gorm:"not null"                                       json:"start_time"`
	EndTime   time.Time `gorm:"not null"                                       json:"end_time"`

	Timestamps
}

func (OncallSchedule) TableName() string { return "oncall_schedules" }

type LDAPGroupRole struct {
	ID        string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	LDAPGroup string `gorm:"type:varchar(255);not null;uniqueIndex"         json:"ldap_group"`
	RoleName  string `gorm:"type:varchar(32);not null"                      json:"role_name"`

	Timestamps
}

func (LDAPGroupRole) TableName() string { return "ldap_group_roles" }

type AlertRoute struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"not null"                                       json:"name"`
	Priority    int            `gorm:"default:0"                                      json:"priority"`
	Matchers    datatypes.JSON `gorm:"type:jsonb;not null"                            json:"matchers"`
	GroupBy     datatypes.JSON `gorm:"type:jsonb"                                     json:"group_by"`
	ChannelIDs  datatypes.JSON `gorm:"type:jsonb"                                     json:"channel_ids"`
	IsEnabled   bool           `gorm:"default:true"                                   json:"is_enabled"`
	Description string         `json:"description"`

	Timestamps
}

func (AlertRoute) TableName() string { return "alert_routes" }
