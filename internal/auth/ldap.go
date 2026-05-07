package auth

import (
	"context"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

// LDAPConfig holds LDAP connection and mapping parameters (stored in DB, loaded at runtime).
type LDAPConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	UseTLS          bool   `json:"use_tls"`
	BindDN          string `json:"bind_dn"`
	BindPassword    string `json:"bind_password"`
	BaseDN          string `json:"base_dn"`
	UserFilter      string `json:"user_filter"`
	AttrUsername    string `json:"attr_username"`
	AttrEmail       string `json:"attr_email"`
	AttrDisplayName string `json:"attr_display_name"`
	GroupBaseDN     string `json:"group_base_dn"`
	GroupFilter     string `json:"group_filter"`
	GroupAttr       string `json:"group_attr"`
	SyncInterval    string `json:"sync_interval"`
}

type LDAPAuth struct {
	db  *gorm.DB
	cfg *LDAPConfig
}

func NewLDAPAuth(db *gorm.DB, cfg *LDAPConfig) *LDAPAuth {
	return &LDAPAuth{db: db, cfg: cfg}
}

// Authenticate performs LDAP bind authentication and syncs user roles.
// Phase 4: implement with go-ldap.
func (a *LDAPAuth) Authenticate(ctx context.Context, username, password string) (*model.User, error) {
	log.Warn().Msg("LDAP authentication not yet implemented")
	return nil, ErrNotImplemented
}

// SyncUser synchronises LDAP user attributes and group-role mappings to the local database.
func (a *LDAPAuth) SyncUser(ctx context.Context, username string) error {
	log.Warn().Msg("LDAP user sync not yet implemented")
	return ErrNotImplemented
}
