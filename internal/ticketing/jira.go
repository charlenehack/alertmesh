package ticketing

import (
	"context"

	"github.com/rs/zerolog/log"
)

// JiraClient integrates with Jira for automatic ticket creation.
type JiraClient struct {
	BaseURL  string
	Username string
	Token    string
}

func NewJiraClient(baseURL, username, token string) *JiraClient {
	return &JiraClient{BaseURL: baseURL, Username: username, Token: token}
}

// CreateTicket creates a Jira issue linked to an incident.
func (c *JiraClient) CreateTicket(ctx context.Context, title, description, severity string) (string, error) {
	log.Debug().Str("integration", "jira").Msg("create ticket (stub)")
	return "", nil
}
