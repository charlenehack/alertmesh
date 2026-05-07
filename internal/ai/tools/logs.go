package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LogsTool searches logs via OpenSearch / Elasticsearch HTTP API.
//
// This is the primary log analysis tool. In a typical deployment, BOTH
// application logs AND system logs (syslog, journal, dmesg, audit) are
// collected into OpenSearch, eliminating the need for a separate CMDB.
//
// The AI agent can query:
//   - Application error logs (by service, namespace, pod)
//   - System logs (kernel OOM, disk errors, network issues)
//   - Deployment/change logs (CI/CD pipeline output)
//   - Security audit logs
//
// Input format (JSON):
//
//	{
//	  "index": "app-logs-*",
//	  "query": "error AND service:order-api",
//	  "time_range": "1h",
//	  "size": 50
//	}
//
// Alternatively, a plain text query string is accepted (searches all indices).
type LogsTool struct {
	BaseURL string
}

func NewLogsTool(baseURL string) *LogsTool {
	return &LogsTool{BaseURL: baseURL}
}

func (t *LogsTool) Name() string { return "logs_search" }
func (t *LogsTool) Description() string {
	return `Search application logs and system logs via OpenSearch/Elasticsearch. Input JSON: {"index": "app-logs-*", "query": "error AND timeout", "time_range": "1h", "size": 50}. Index examples: "app-logs-*" for application logs, "syslog-*" for system logs (kernel, dmesg, journal), "deploy-logs-*" for deployment/change records. time_range and size are optional (defaults: 1h, 30). A plain text query string is also accepted.`
}

type logsInput struct {
	Index     string `json:"index"`
	Query     string `json:"query"`
	TimeRange string `json:"time_range"`
	Size      int    `json:"size"`
}

func (t *LogsTool) Call(ctx context.Context, input string) (string, error) {
	if t.BaseURL == "" {
		return "logs_search: OpenSearch/Elasticsearch URL not configured. Set ALERTMESH_OPENSEARCH_URL.", nil
	}

	var li logsInput
	if err := json.Unmarshal([]byte(input), &li); err != nil {
		li.Query = strings.TrimSpace(input)
	}
	if li.Query == "" {
		return "logs_search: query is required", nil
	}

	if li.Index == "" {
		li.Index = "*"
	}
	if li.Size <= 0 || li.Size > 100 {
		li.Size = 30
	}
	if li.TimeRange == "" {
		li.TimeRange = "1h"
	}

	dur, err := time.ParseDuration(li.TimeRange)
	if err != nil {
		dur = 1 * time.Hour
	}

	// Build OpenSearch/ES query
	esQuery := buildESQuery(li.Query, dur, li.Size)
	queryJSON, _ := json.Marshal(esQuery)

	apiURL := fmt.Sprintf("%s/%s/_search", strings.TrimRight(t.BaseURL, "/"), li.Index)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, apiURL, bytes.NewReader(queryJSON))
	if err != nil {
		return "", fmt.Errorf("logs_search: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("logs_search: request failed: %v", err), nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("logs_search: OpenSearch returned %d: %s", resp.StatusCode, truncate(string(body), 500)), nil
	}

	return formatESResponse(body), nil
}

func buildESQuery(queryStr string, timeRange time.Duration, size int) map[string]any {
	now := time.Now()
	from := now.Add(-timeRange)

	return map[string]any{
		"size": size,
		"sort": []any{
			map[string]any{"@timestamp": map[string]string{"order": "desc"}},
		},
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{
						"query_string": map[string]any{
							"query": queryStr,
						},
					},
				},
				"filter": []any{
					map[string]any{
						"range": map[string]any{
							"@timestamp": map[string]string{
								"gte": from.Format(time.RFC3339),
								"lte": now.Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
		"_source": []string{"@timestamp", "message", "level", "logger", "host", "service", "namespace", "pod", "container"},
	}
}

func formatESResponse(raw []byte) string {
	var resp struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source map[string]any `json:"_source"`
				Index  string         `json:"_index"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return truncate(string(raw), 6000)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d logs (showing %d):\n\n", resp.Hits.Total.Value, len(resp.Hits.Hits)))

	for i, hit := range resp.Hits.Hits {
		ts, _ := hit.Source["@timestamp"].(string)
		msg, _ := hit.Source["message"].(string)
		level, _ := hit.Source["level"].(string)
		host, _ := hit.Source["host"].(string)
		svc, _ := hit.Source["service"].(string)

		sb.WriteString(fmt.Sprintf("[%d] %s", i+1, ts))
		if level != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", level))
		}
		if host != "" {
			sb.WriteString(fmt.Sprintf(" host=%s", host))
		}
		if svc != "" {
			sb.WriteString(fmt.Sprintf(" service=%s", svc))
		}
		sb.WriteString(fmt.Sprintf(" index=%s\n", hit.Index))
		if msg != "" {
			if len(msg) > 500 {
				msg = msg[:500] + "..."
			}
			sb.WriteString(fmt.Sprintf("   %s\n", msg))
		}
		sb.WriteString("\n")
	}

	result := sb.String()
	return truncate(result, 8000)
}
