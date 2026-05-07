package model

import (
	"time"
)

type User struct {
	ID           string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Username     string     `gorm:"uniqueIndex;not null"                           json:"username"`
	Email        string     `gorm:"uniqueIndex"                                    json:"email"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `gorm:"type:varchar(255)"                              json:"-"`
	Source       string     `gorm:"not null"                                       json:"source"` // local/ldap/oidc
	ExternalID   string     `gorm:"index"                                          json:"external_id"`
	Roles        []*Role    `gorm:"many2many:user_roles"                           json:"roles,omitempty"`
	IsActive     bool       `gorm:"default:true"                                   json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`

	Timestamps
}

func (User) TableName() string { return "users" }
