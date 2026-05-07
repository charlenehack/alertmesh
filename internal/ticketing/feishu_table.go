package ticketing

import (
	"context"

	"github.com/rs/zerolog/log"
)

// FeishuTableClient integrates with Feishu Bitable (multi-dimensional tables) for ticket management.
type FeishuTableClient struct {
	AppID     string
	AppSecret string
	TableID   string
}

func NewFeishuTableClient(appID, appSecret, tableID string) *FeishuTableClient {
	return &FeishuTableClient{AppID: appID, AppSecret: appSecret, TableID: tableID}
}

// CreateRecord creates a record in Feishu Bitable linked to an incident.
func (c *FeishuTableClient) CreateRecord(ctx context.Context, title, description, severity string) (string, error) {
	log.Debug().Str("integration", "feishu_table").Msg("create record (stub)")
	return "", nil
}
