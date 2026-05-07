package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MetricsTool queries Prometheus HTTP API for metrics data via PromQL.
//
// The AI agent can use this tool to:
//   - Query instant metrics: current CPU, memory, disk, network usage
//   - Query range data: trend over the past N minutes/hours
//   - Check alerting rules status
//   - Correlate metrics with incident timeframes
//
// Input format (JSON):
//
//	{"query": "<PromQL expression>", "time": "optional RFC3339", "range": "optional duration like 1h"}
//
// If "range" is provided, a range query is executed; otherwise an instant query.
type MetricsTool struct {
	BaseURL string
}

func NewMetricsTool(baseURL string) *MetricsTool {
	return &MetricsTool{BaseURL: baseURL}
}

func (t *MetricsTool) Name() string { return "metrics_query" }
func (t *MetricsTool) Description() string {
	return `Query Prometheus metrics via PromQL. Input must be JSON: {"query": "up{job=\"node\"}", "range": "1h"} where "range" is optional (omit for instant query). Use this to check CPU, memory, disk, network, container metrics, error rates, latency percentiles, etc.`
}

type metricsInput struct {
	Query string `json:"query"`
	Time  string `json:"time"`
	Range string `json:"range"`
}

func (t *MetricsTool) Call(ctx context.Context, input string) (string, error) {
	if t.BaseURL == "" {
		return "metrics_query: Prometheus URL not configured. Set ALERTMESH_PROMETHEUS_URL.", nil
	}

	var mi metricsInput
	if err := json.Unmarshal([]byte(input), &mi); err != nil {
		mi.Query = strings.TrimSpace(input)
	}
	if mi.Query == "" {
		return "metrics_query: query is required", nil
	}

	if mi.Range != "" {
		return t.rangeQuery(ctx, mi)
	}
	return t.instantQuery(ctx, mi)
}

func (t *MetricsTool) instantQuery(ctx context.Context, mi metricsInput) (string, error) {
	params := url.Values{"query": {mi.Query}}
	if mi.Time != "" {
		params.Set("time", mi.Time)
	}

	apiURL := fmt.Sprintf("%s/api/v1/query?%s", strings.TrimRight(t.BaseURL, "/"), params.Encode())
	return t.doRequest(ctx, apiURL)
}

func (t *MetricsTool) rangeQuery(ctx context.Context, mi metricsInput) (string, error) {
	dur, err := time.ParseDuration(mi.Range)
	if err != nil {
		return fmt.Sprintf("metrics_query: invalid range duration %q: %v", mi.Range, err), nil
	}

	end := time.Now()
	start := end.Add(-dur)

	step := dur / 60
	if step < 15*time.Second {
		step = 15 * time.Second
	}

	params := url.Values{
		"query": {mi.Query},
		"start": {start.Format(time.RFC3339)},
		"end":   {end.Format(time.RFC3339)},
		"step":  {fmt.Sprintf("%.0f", step.Seconds())},
	}

	apiURL := fmt.Sprintf("%s/api/v1/query_range?%s", strings.TrimRight(t.BaseURL, "/"), params.Encode())
	return t.doRequest(ctx, apiURL)
}

func (t *MetricsTool) doRequest(ctx context.Context, apiURL string) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("metrics_query: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("metrics_query: request failed: %v", err), nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("metrics_query: Prometheus returned %d: %s", resp.StatusCode, truncate(string(body), 500)), nil
	}

	return truncateJSON(body, 8000), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

func truncateJSON(raw []byte, maxLen int) string {
	// Try to compact the JSON for readability within token limits
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return truncate(string(raw), maxLen)
	}

	// Extract just status + data for conciseness
	result := map[string]any{
		"status": parsed["status"],
	}
	if data, ok := parsed["data"]; ok {
		result["data"] = data
	}

	compact, err := json.Marshal(result)
	if err != nil {
		return truncate(string(raw), maxLen)
	}
	return truncate(string(compact), maxLen)
}
