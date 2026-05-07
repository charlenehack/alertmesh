package store

import (
	"github.com/rs/zerolog/log"

	"github.com/kuzane/alertmesh/internal/config"
)

// NewRedis initialises the Redis client when ALERTMESH_REDIS_ENABLED=true.
// Phase 3: replace with go-redis/v9 implementation.
func NewRedis(cfg *config.Config) error {
	if !cfg.RedisEnabled {
		log.Debug().Msg("redis disabled, skipping")
		return nil
	}

	log.Info().Str("addr", cfg.RedisAddr).Msg("redis enabled (stub)")
	// TODO: implement go-redis/v9 client
	return nil
}
