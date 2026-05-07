// Package sysconfig provides an encrypted key/value store backed by the
// system_configs table.  It is the single source of truth for secrets (JWT
// signing key, RSA private key, auth provider credentials) and public runtime
// metadata (version, RSA public key) that must not live in plain-text
// environment variables.
package sysconfig

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/auth"
	cfgcrypto "github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/internal/version"
)

// Well-known keys stored in system_configs.
const (
	// ── system metadata (plain, readable via /configs) ──────────────────────
	KeySystemInitialized = "system.initialized"
	KeyVersion           = "system.version"
	KeyRSAPublicKey      = "system.rsa_public_key"

	// ── secrets (AES-256-GCM encrypted with the master key) ─────────────────
	KeyJWTSecret    = "security.jwt_secret" //nolint:gosec // setting key, not the secret value
	KeyRSAPrivatKey = "security.rsa_private_key"

	// ── auth configuration ───────────────────────────────────────────────────
	KeyAuthMode = "auth.mode" // "local" | "ldap" | "oidc"
	KeyAuthLDAP = "auth.ldap" // JSON, encrypted
	KeyAuthOIDC = "auth.oidc" // JSON, encrypted
)

// Service wraps the system_configs table with plain and encrypted access.
type Service struct {
	db        *gorm.DB
	masterKey string // base64(32 bytes) from ALERTMESH_ENCRYPTION_KEY
}

// NewService creates a Service.  masterKey must be the base64-encoded 32-byte
// AES-256 master encryption key (ALERTMESH_ENCRYPTION_KEY).
func NewService(db *gorm.DB, masterKey string) *Service {
	return &Service{db: db, masterKey: masterKey}
}

// ─── plain get / set ─────────────────────────────────────────────────────────

// Get returns the stored value for key (raw, not decrypted).
func (s *Service) Get(ctx context.Context, key string) (string, error) {
	var row model.SystemConfig
	if err := s.db.WithContext(ctx).Where("key = ?", key).First(&row).Error; err != nil {
		return "", err
	}
	return row.Value, nil
}

// Set upserts a plain value with an optional description.
func (s *Service) Set(ctx context.Context, key, value string, desc ...string) error {
	row := model.SystemConfig{Key: key, Value: value}
	if len(desc) > 0 {
		row.Description = desc[0]
	}
	return s.db.WithContext(ctx).Save(&row).Error
}

// ─── encrypted get / set ─────────────────────────────────────────────────────

// GetSecret retrieves and decrypts a sensitive value.
func (s *Service) GetSecret(ctx context.Context, key string) (string, error) {
	enc, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	plain, err := cfgcrypto.Decrypt(enc, s.masterKey)
	if err != nil {
		return "", fmt.Errorf("sysconfig: decrypt %q: %w", key, err)
	}
	return plain, nil
}

// SetSecret encrypts value and stores it.
func (s *Service) SetSecret(ctx context.Context, key, value string, desc ...string) error {
	enc, err := cfgcrypto.Encrypt(value, s.masterKey)
	if err != nil {
		return fmt.Errorf("sysconfig: encrypt %q: %w", key, err)
	}
	return s.Set(ctx, key, enc, desc...)
}

// ─── domain helpers ──────────────────────────────────────────────────────────

// JWTSecret returns the JWT signing key (decrypted).
func (s *Service) JWTSecret(ctx context.Context) (string, error) {
	return s.GetSecret(ctx, KeyJWTSecret)
}

// RSAPrivateKeyPEM returns the PKCS#8 PEM-encoded RSA private key (decrypted).
// Used by providers.go to call auth.InitRSAFromPEM at startup.
func (s *Service) RSAPrivateKeyPEM(ctx context.Context) (string, error) {
	return s.GetSecret(ctx, KeyRSAPrivatKey)
}

// AuthMode returns the configured authentication mode.  Defaults to "local".
func (s *Service) AuthMode(ctx context.Context) string {
	v, err := s.Get(ctx, KeyAuthMode)
	if err != nil {
		return "local"
	}
	return v
}

// SetAuthMode updates the authentication mode and optionally stores the
// provider-specific config JSON (encrypted) for ldap/oidc.
func (s *Service) SetAuthMode(ctx context.Context, mode, providerConfigJSON string) error {
	if err := s.Set(ctx, KeyAuthMode, mode); err != nil {
		return err
	}
	if providerConfigJSON == "" {
		return nil
	}
	var key string
	switch mode {
	case "ldap":
		key = KeyAuthLDAP
	case "oidc":
		key = KeyAuthOIDC
	default:
		return nil
	}
	return s.SetSecret(ctx, key, providerConfigJSON)
}

// AuthProviderConfig retrieves and decrypts the JSON config for the given mode.
func (s *Service) AuthProviderConfig(ctx context.Context, mode string) (string, error) {
	var key string
	switch mode {
	case "ldap":
		key = KeyAuthLDAP
	case "oidc":
		key = KeyAuthOIDC
	default:
		return "", fmt.Errorf("sysconfig: no provider config for mode %q", mode)
	}
	return s.GetSecret(ctx, key)
}

// ─── first-boot bootstrap ────────────────────────────────────────────────────

// Bootstrap performs system initialisation on every startup.
// It is safe to call repeatedly – each step is idempotent.
//
// Steps run on EVERY startup:
//   - Refresh system.version to the running binary's version.
//   - Ensure the RSA-2048 key pair exists (generate+store only if missing).
//
// Steps run only on FIRST startup (when system.initialized is absent):
//   - Generate a random 32-byte JWT signing secret (AES-256-GCM encrypted).
//   - Set auth.mode = "local".
//   - Write system.initialized = "true".
func (s *Service) Bootstrap(ctx context.Context) error {
	// ── Always: refresh version ───────────────────────────────────────────────
	if err := s.Set(ctx, KeyVersion, version.String(),
		"Running AlertMesh version (semver)"); err != nil {
		log.Warn().Err(err).Msg("sysconfig: failed to write system.version")
	}

	// ── Always: ensure RSA key pair exists (idempotent) ──────────────────────
	// This runs on every startup so that existing deployments that were
	// bootstrapped before RSA support was added automatically get a key pair.
	if err := s.ensureRSAKeyPair(ctx); err != nil {
		return err
	}

	// ── First-boot only ───────────────────────────────────────────────────────
	var existing model.SystemConfig
	err := s.db.WithContext(ctx).
		Where("key = ?", KeySystemInitialized).
		First(&existing).Error

	if err == nil {
		return nil // already initialised
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("sysconfig bootstrap: check init flag: %w", err)
	}

	// Generate a random 32-byte JWT secret.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("sysconfig bootstrap: generate jwt secret: %w", err)
	}
	jwtSecret := base64.StdEncoding.EncodeToString(raw)
	if err := s.SetSecret(ctx, KeyJWTSecret, jwtSecret,
		"JWT signing secret (AES-256-GCM encrypted)"); err != nil {
		return fmt.Errorf("sysconfig bootstrap: store jwt secret: %w", err)
	}

	if err := s.Set(ctx, KeyAuthMode, "local",
		"Authentication mode: local | ldap | oidc"); err != nil {
		return fmt.Errorf("sysconfig bootstrap: set auth mode: %w", err)
	}
	if err := s.Set(ctx, KeySystemInitialized, "true",
		"Marker written once after successful first-boot bootstrap"); err != nil {
		return fmt.Errorf("sysconfig bootstrap: mark initialized: %w", err)
	}

	log.Info().
		Str("version", version.String()).
		Msg("system bootstrap: first-boot completed; jwt_secret stored encrypted")
	return nil
}

// ensureRSAKeyPair generates and stores an RSA-2048 key pair if one does not
// already exist in system_configs.  Calling this multiple times is safe.
func (s *Service) ensureRSAKeyPair(ctx context.Context) error {
	err := s.db.WithContext(ctx).
		Where("key = ?", KeyRSAPrivatKey).
		First(&model.SystemConfig{}).Error

	if err == nil {
		return nil // key pair already present
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("sysconfig: check rsa key: %w", err)
	}

	privPEM, pubPEM, err := auth.GenerateRSAKeyPair()
	if err != nil {
		return fmt.Errorf("sysconfig: generate rsa key pair: %w", err)
	}
	if err := s.Set(ctx, KeyRSAPublicKey, pubPEM,
		"RSA-2048 public key (PEM) – browser uses this to encrypt login passwords"); err != nil {
		return fmt.Errorf("sysconfig: store rsa public key: %w", err)
	}
	if err := s.SetSecret(ctx, KeyRSAPrivatKey, privPEM,
		"RSA-2048 private key (PEM, AES-256-GCM encrypted)"); err != nil {
		return fmt.Errorf("sysconfig: store rsa private key: %w", err)
	}

	log.Info().Msg("sysconfig: rsa-2048 key pair generated and stored in system_configs")
	return nil
}
