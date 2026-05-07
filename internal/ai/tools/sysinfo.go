package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SystemInfoTool queries system-level information by searching system logs
// (syslog, journal, dmesg) collected in OpenSearch/Elasticsearch.
//
// This replaces a traditional CMDB lookup by leveraging the fact that system
// logs contain host metadata, hardware events, kernel messages, and service
// status — sufficient for root cause analysis without a separate CMDB API.
//
// Input format (JSON):
//
//	{"host": "prod-web-01", "category": "kernel|disk|network|oom|service", "time_range": "2h"}
//
// category maps to curated queries:
//   - kernel:  dmesg / kernel panic / hardware errors
//   - disk:    filesystem full, I/O errors, SMART warnings
//   - network: interface down, packet drops, connection refused
//   - oom:     OOM killer, memory pressure
//   - service: systemd unit failures, restart loops
type SystemInfoTool struct {
	LogsTool *LogsTool
}

func NewSystemInfoTool(logsTool *LogsTool) *SystemInfoTool {
	return &SystemInfoTool{LogsTool: logsTool}
}

func (t *SystemInfoTool) Name() string { return "system_info" }
func (t *SystemInfoTool) Description() string {
	return `Query system-level information (kernel, disk, network, OOM, service status) from syslog collected in OpenSearch. Input JSON: {"host": "prod-web-01", "category": "oom", "time_range": "2h"}. Categories: kernel, disk, network, oom, service. This replaces CMDB lookups by searching system logs directly.`
}

type systemInfoInput struct {
	Host      string `json:"host"`
	Category  string `json:"category"`
	TimeRange string `json:"time_range"`
}

var categoryQueries = map[string]string{
	"kernel":  `(kernel OR dmesg OR "hardware error" OR panic OR "machine check" OR "general protection")`,
	"disk":    `("No space left" OR "I/O error" OR "filesystem" OR "read-only" OR "SMART" OR "disk" OR "ext4" OR "xfs")`,
	"network": `("link is not ready" OR "connection refused" OR "connection timed out" OR "packet drop" OR "unreachable" OR "interface" OR "NetworkManager")`,
	"oom":     `("Out of memory" OR "oom-kill" OR "oom_reaper" OR "memory cgroup" OR "Cannot allocate memory" OR "killed process")`,
	"service": `("systemd" OR "Failed to start" OR "entered failed state" OR "Start request repeated" OR "service exited" OR "main process exited")`,
}

func (t *SystemInfoTool) Call(ctx context.Context, input string) (string, error) {
	var si systemInfoInput
	if err := json.Unmarshal([]byte(input), &si); err != nil {
		return "system_info: input must be JSON with host and category fields", nil
	}

	if si.Category == "" {
		si.Category = "kernel"
	}
	if si.TimeRange == "" {
		si.TimeRange = "2h"
	}

	catQuery, ok := categoryQueries[strings.ToLower(si.Category)]
	if !ok {
		return fmt.Sprintf("system_info: unknown category %q, use one of: kernel, disk, network, oom, service", si.Category), nil
	}

	query := catQuery
	if si.Host != "" {
		query = fmt.Sprintf("host:%s AND %s", si.Host, catQuery)
	}

	logsInput, _ := json.Marshal(map[string]any{
		"index":      "syslog-*",
		"query":      query,
		"time_range": si.TimeRange,
		"size":       40,
	})

	return t.LogsTool.Call(ctx, string(logsInput))
}
