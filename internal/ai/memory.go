package ai

import (
	"context"

	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
)

// Memory manages per-incident conversation history for multi-turn AI follow-up.
type Memory struct {
	db *gorm.DB
}

func NewMemory(db *gorm.DB) *Memory {
	return &Memory{db: db}
}

// AddMessage persists a conversation message.
func (m *Memory) AddMessage(ctx context.Context, incidentID, role, content, userID string) error {
	msg := &model.AIConversation{
		IncidentID: incidentID,
		Role:       role,
		Content:    content,
		UserID:     userID,
	}
	return m.db.WithContext(ctx).Create(msg).Error
}

// GetHistory retrieves the conversation history for an incident.
func (m *Memory) GetHistory(ctx context.Context, incidentID string) ([]model.AIConversation, error) {
	var messages []model.AIConversation
	err := m.db.WithContext(ctx).
		Where("incident_id = ?", incidentID).
		Order("created_at ASC").
		Find(&messages).Error
	return messages, err
}
