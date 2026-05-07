package auth

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

var ErrNotImplemented = errors.New("not implemented")

type OIDCAuth struct {
	db *gorm.DB
}

func NewOIDCAuth(db *gorm.DB) *OIDCAuth {
	return &OIDCAuth{db: db}
}

// Authenticate validates an OIDC token and returns the corresponding user.
// Phase 4: implement with coreos/go-oidc.
func (a *OIDCAuth) Authenticate(ctx context.Context, idToken string) (*model.User, error) {
	log.Warn().Msg("OIDC authentication not yet implemented")
	return nil, ErrNotImplemented
}
