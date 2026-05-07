package tools

import (
	"github.com/tmc/langchaingo/tools"
)

// ToolConfig holds the external service URLs needed by AI tools.
// Only Prometheus + OpenSearch/ES are required for full root cause analysis.
type ToolConfig struct {
	PrometheusURL string // e.g. "http://prometheus:9090"
	OpenSearchURL string // e.g. "http://opensearch:9200"
}

// AsLangchainTools returns all AI analysis tools as langchaingo-compatible Tool values.
//
// The tool set is intentionally minimal — only two external dependencies:
//
//   - Prometheus: for PromQL metrics queries (CPU, memory, latency, error rates, etc.)
//   - OpenSearch/ES: for ALL log-based queries (app logs, system logs, deploy logs, runbooks)
//
// The system_info, changes_query, and runbook_search tools are implemented on
// top of the logs_search tool with curated queries, eliminating the need for
// separate CMDB, deployment, or knowledge base APIs.
func AsLangchainTools(cfgs ...ToolConfig) []tools.Tool {
	var cfg ToolConfig
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	logsTool := NewLogsTool(cfg.OpenSearchURL)

	return []tools.Tool{
		NewMetricsTool(cfg.PrometheusURL),
		logsTool,
		NewSystemInfoTool(logsTool),
		NewChangesTool(logsTool),
		NewRunbookTool(logsTool),
	}
}
