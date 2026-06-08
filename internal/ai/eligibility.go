package ai

import (
	"context"

	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

// ShouldRun answers: "may this incident use AI at all (manual button + UI)?"
// It does not imply automatic enqueue — see ShouldAutoEnqueue.
// Returns false (and never panics) on any of:
//
//   - empty dsID (legacy incident or adapter that didn't carry a source id)
//   - data source row missing / soft-deleted
//   - data source disabled (`is_enabled=false`)
//   - operator hasn't flipped the per-source `ai_enabled` switch
//
// Used by incident.Service.createIncident (to set ai_status=disabled vs pending)
// and router.aiHandler.trigger (manual button).
func ShouldRun(ctx context.Context, db *gorm.DB, dsID string) bool {
	if dsID == "" {
		return false
	}
	var ds model.DataSource
	if err := db.WithContext(ctx).
		Select("kind", "ai_enabled", "is_enabled").
		Where("id = ?", dsID).
		First(&ds).Error; err != nil {
		return false
	}
	return ds.IsEnabled && ds.AIEnabled
}

// ShouldAutoEnqueue is true when a brand-new incident from this source should
// immediately create an ai_tasks row (operator did not click "触发 AI 分析").
// Requires the same enabled / ai_enabled gate as ShouldRun plus
// data_sources.ai_auto_trigger = true.
func ShouldAutoEnqueue(ctx context.Context, db *gorm.DB, dsID string) bool {
	if dsID == "" {
		return false
	}
	var ds model.DataSource
	if err := db.WithContext(ctx).
		Select("kind", "ai_enabled", "is_enabled", "ai_auto_trigger").
		Where("id = ?", dsID).
		First(&ds).Error; err != nil {
		return false
	}
	return ds.IsEnabled && ds.AIEnabled && ds.AIAutoTrigger
}
