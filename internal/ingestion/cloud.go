package ingestion

import (
	"encoding/json"
	"time"
)

// CloudRDSAdapter normalises cloud RDS slow query alerts.
type CloudRDSAdapter struct {
	SlowQueryWarningThreshold  float64 // seconds, default 5
	SlowQueryCriticalThreshold float64 // seconds, default 30
}

func NewCloudRDSAdapter() *CloudRDSAdapter {
	return &CloudRDSAdapter{
		SlowQueryWarningThreshold:  5,
		SlowQueryCriticalThreshold: 30,
	}
}

func (a *CloudRDSAdapter) Name() string { return "cloud-rds" }

type cloudRDSPayload struct {
	DBInstanceID   string  `json:"db_instance_id"`
	RegionID       string  `json:"region_id"`
	DBName         string  `json:"db_name"`
	Engine         string  `json:"engine"`
	SQLText        string  `json:"sql_text"`
	ExecutionTime  float64 `json:"execution_time"`
	QueryStartTime string  `json:"query_start_time"`
}

func (a *CloudRDSAdapter) Adapt(payload []byte) ([]RawAlert, error) {
	var p cloudRDSPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}

	severity := "info"
	switch {
	case p.ExecutionTime >= a.SlowQueryCriticalThreshold:
		severity = "critical"
	case p.ExecutionTime >= a.SlowQueryWarningThreshold:
		severity = "warning"
	}

	sqlTrunc := p.SQLText
	if len(sqlTrunc) > 500 {
		sqlTrunc = sqlTrunc[:500]
	}

	labels := map[string]string{
		"alertname":   "SlowQuery",
		"severity":    severity,
		"source":      "cloud-rds",
		"db_instance": p.DBInstanceID,
		"region":      p.RegionID,
		"database":    p.DBName,
		"db_engine":   p.Engine,
	}

	startsAt, _ := time.Parse(time.RFC3339, p.QueryStartTime)
	if startsAt.IsZero() {
		startsAt = time.Now()
	}

	fp := ComputeFingerprint(map[string]string{
		"db_instance": p.DBInstanceID,
		"database":    p.DBName,
		"sql":         sqlTrunc,
	})

	return []RawAlert{{
		Source:      "cloud-rds",
		Fingerprint: fp,
		Labels:      labels,
		Annotations: map[string]string{"description": sqlTrunc},
		StartsAt:    startsAt,
		Status:      "firing",
		RawPayload:  payload,
	}}, nil
}
