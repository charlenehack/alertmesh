package incident

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, inc *model.Incident) error {
	return r.db.WithContext(ctx).Create(inc).Error
}

func (r *Repository) FindByID(ctx context.Context, id string) (*model.Incident, error) {
	var inc model.Incident
	err := r.db.WithContext(ctx).
		Preload("Alerts").
		Preload("Timeline").
		First(&inc, "id = ?", id).Error
	return &inc, err
}

// FindActiveByGroupKey is the v2 lookup used by HandleAlertGroup.  It first
// looks for an incident already in open / ack / in_progress for this
// group_key (the "append to existing" case).  Only when nothing matches
// does it fall back to recently-resolved incidents whose resolved_at is
// still inside the configured reopen window — these get reopened instead
// of creating a brand-new row.  The boolean return tells the caller which
// branch hit ("reopen" = true means this incident is currently resolved
// and the caller should flip it back to open before appending).  Returns
// gorm.ErrRecordNotFound when neither query matches.
//
// reopenWindow ≤ 0 disables the reopen branch entirely (legacy behaviour).
func (r *Repository) FindActiveByGroupKey(
	ctx context.Context, groupKey string, reopenWindow time.Duration,
) (*model.Incident, bool, error) {
	var inc model.Incident

	openErr := r.db.WithContext(ctx).
		Where("group_key = ? AND status IN ?", groupKey, []string{
			model.IncidentStatusOpen,
			model.IncidentStatusAck,
			model.IncidentStatusInProgress,
		}).
		Order("opened_at DESC").
		First(&inc).Error
	if openErr == nil {
		return &inc, false, nil
	}
	if !errors.Is(openErr, gorm.ErrRecordNotFound) {
		return nil, false, openErr
	}

	if reopenWindow <= 0 {
		return nil, false, gorm.ErrRecordNotFound
	}

	cutoff := time.Now().Add(-reopenWindow)
	if err := r.db.WithContext(ctx).
		Where("group_key = ? AND status = ? AND resolved_at >= ?",
			groupKey, model.IncidentStatusResolved, cutoff,
		).
		Order("resolved_at DESC").
		First(&inc).Error; err != nil {
		return nil, false, err
	}
	return &inc, true, nil
}

// FindLatestClosedByGroupKey returns the most recent resolved/closed
// incident for the given group_key.  Used by createIncident to populate
// parent_incident_id when the previous occurrence's reopen window has
// already lapsed (so the UI can render a "延续自 #xxx" link).  Returns
// gorm.ErrRecordNotFound when this group_key has never produced an
// incident before.
func (r *Repository) FindLatestClosedByGroupKey(ctx context.Context, groupKey string) (*model.Incident, error) {
	var inc model.Incident
	err := r.db.WithContext(ctx).
		Where("group_key = ? AND status IN ?", groupKey, []string{
			model.IncidentStatusResolved,
			model.IncidentStatusClosed,
		}).
		Order("COALESCE(resolved_at, updated_at) DESC").
		First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

func (r *Repository) Update(ctx context.Context, inc *model.Incident) error {
	return r.db.WithContext(ctx).Save(inc).Error
}

func (r *Repository) List(ctx context.Context, offset, limit int) ([]model.Incident, int64, error) {
	var incidents []model.Incident
	var total int64

	q := r.db.WithContext(ctx).Model(&model.Incident{})
	q.Count(&total)

	err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&incidents).Error
	return incidents, total, err
}

func (r *Repository) AddAlerts(ctx context.Context, alerts []model.Alert) error {
	return r.db.WithContext(ctx).Create(&alerts).Error
}
