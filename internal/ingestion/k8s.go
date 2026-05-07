package ingestion

import (
	"github.com/rs/zerolog/log"

	"github.com/kuzane/alertmesh/internal/config"
)

// StartK8sInformer starts a K8s Events informer that converts Warning events into RawAlerts.
// Phase 3: implement with client-go.
func StartK8sInformer(cfg *config.Config, pipeline func(RawAlert)) {
	if !cfg.K8sEnabled {
		return
	}
	log.Info().
		Str("cluster", cfg.K8sClusterName).
		Msg("k8s events informer started (stub)")
	// TODO: implement client-go informer for Warning events
}
