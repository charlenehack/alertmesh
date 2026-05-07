package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// RunbookTool searches runbooks and knowledge base articles stored in
// OpenSearch/Elasticsearch.
//
// Runbooks can be indexed into a dedicated "runbooks" index, or the tool
// can search across all indices for documents tagged as runbooks/playbooks.
//
// Input format (JSON):
//
//	{"query": "high cpu linux", "index": "runbooks"}
//
// A plain text query string is also accepted.
type RunbookTool struct {
	LogsTool *LogsTool
}

func NewRunbookTool(logsTool *LogsTool) *RunbookTool {
	return &RunbookTool{LogsTool: logsTool}
}

func (t *RunbookTool) Name() string { return "runbook_search" }
func (t *RunbookTool) Description() string {
	return `Search runbooks and knowledge base articles for remediation procedures. Input JSON: {"query": "OOM kill remediation"} or plain text query. Searches the runbooks index in OpenSearch for matching SOPs, playbooks, and troubleshooting guides.`
}

type runbookInput struct {
	Query string `json:"query"`
	Index string `json:"index"`
}

func (t *RunbookTool) Call(ctx context.Context, input string) (string, error) {
	var ri runbookInput
	if err := json.Unmarshal([]byte(input), &ri); err != nil {
		ri.Query = input
	}
	if ri.Query == "" {
		return "runbook_search: query is required", nil
	}
	if ri.Index == "" {
		ri.Index = "runbooks"
	}

	logsInput, _ := json.Marshal(map[string]any{
		"index":      ri.Index,
		"query":      fmt.Sprintf(`(%s) AND (type:runbook OR type:playbook OR type:sop OR _index:runbooks)`, ri.Query),
		"time_range": "8760h", // 1 year — runbooks are long-lived docs
		"size":       10,
	})

	return t.LogsTool.Call(ctx, string(logsInput))
}
