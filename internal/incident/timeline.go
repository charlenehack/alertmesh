package incident

import (
	"context"

	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

type TimelineService struct {
	db *gorm.DB
}

func NewTimelineService(db *gorm.DB) *TimelineService {
	return &TimelineService{db: db}
}

func (s *TimelineService) Record(ctx context.Context, entry *model.IncidentTimeline) error {
	return s.db.WithContext(ctx).Create(entry).Error
}

func (s *TimelineService) ListByIncident(ctx context.Context, incidentID string) ([]model.IncidentTimeline, error) {
	var entries []model.IncidentTimeline
	err := s.db.WithContext(ctx).
		Where("incident_id = ?", incidentID).
		Order("created_at ASC").
		Find(&entries).Error
	return entries, err
}
