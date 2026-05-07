package ingestion

import (
	"context"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/config"
)

// StartKafka spins up the production Kafka consumer fleet.  alertmesh
// has NO Kafka producer / sink path — this function is the single entry
// to the consumer link.  Pure DB-driven: brokers / topic / filter /
// mapping / SASL / TLS / rate-limit 全部按行存在 data_sources 中，
// per-row CRUD triggers a debounced reload via pg_notify('data_source_event').
// Empty registry = zero-Reader no-op.  没有 env 开关，调用方只需直接
// 调用本函数即可；要"暂停"消费时把对应 data_sources.is_enabled 置 false
// 或删除行即可在亚秒内热生效。
//
// Returns the manager so callers (mainly cmd/alertmesh) can hold a
// reference for graceful shutdown via the parent context.
func StartKafka(ctx context.Context, cfg *config.Config, db *gorm.DB, pipeline func(RawAlert)) *KafkaManager {
	mgr := NewKafkaManager(db, cfg, pipeline)
	mgr.Start(ctx)
	log.Info().Str("component", "kafka").Msg("kafka manager started (per-row readers driven by data_sources)")
	return mgr
}
