package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ChangesTool queries recent deployment and configuration changes by searching
// deployment logs collected in OpenSearch/Elasticsearch.
//
// In a standard setup, CI/CD pipelines (Jenkins, GitLab CI, ArgoCD, etc.) push
// deployment events to a dedicated log index (e.g. "deploy-logs-*"). This tool
// queries those logs to find changes that correlate with the incident timeframe.
//
// Input format (JSON):
//
//	{"service": "order-api", "namespace": "production", "time_range": "6h"}
//
// All fields are optional; omitting them broadens the search.
type ChangesTool struct {
	LogsTool *LogsTool
}

func NewChangesTool(logsTool *LogsTool) *ChangesTool {
	return &ChangesTool{LogsTool: logsTool}
}

func (t *ChangesTool) Name() string { return "changes_query" }
func (t *ChangesTool) Description() string {
	return `Query recent deployments and configuration changes from deployment logs in OpenSearch. Input JSON: {"service": "order-api", "namespace": "production", "time_range": "6h"}. All fields optional. Searches deploy-logs-* and app-logs-* indices for deployment, rollout, config change, and release events.`
}

type changesInput struct {
	Service   string `json:"service"`
	Namespace string `json:"namespace"`
	TimeRange string `json:"time_range"`
}

func (t *ChangesTool) Call(ctx context.Context, input string) (string, error) {
	var ci changesInput
	if err := json.Unmarshal([]byte(input), &ci); err != nil {
		ci.Service = input
	}
	if ci.TimeRange == "" {
		ci.TimeRange = "6h"
	}

	// Build a query that catches common deployment/change event patterns
	queryParts := []string{
		`("deploy" OR "rollout" OR "release" OR "upgrade" OR "restart" OR "config change" OR "helm" OR "kubectl apply" OR "argocd" OR "pipeline")`,
	}
	if ci.Service != "" {
		queryParts = append(queryParts, fmt.Sprintf("service:%s", ci.Service))
	}
	if ci.Namespace != "" {
		queryParts = append(queryParts, fmt.Sprintf("namespace:%s", ci.Namespace))
	}

	query := queryParts[0]
	for _, p := range queryParts[1:] {
		query += " AND " + p
	}

	logsInput, _ := json.Marshal(map[string]any{
		"index":      "deploy-logs-*,app-logs-*",
		"query":      query,
		"time_range": ci.TimeRange,
		"size":       30,
	})

	return t.LogsTool.Call(ctx, string(logsInput))
}
