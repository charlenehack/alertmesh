package ingestion

import (
	"encoding/json"
	"time"
)

// AlertmanagerAdapter normalises Alertmanager webhook payloads.
type AlertmanagerAdapter struct{}

func NewAlertmanagerAdapter() *AlertmanagerAdapter { return &AlertmanagerAdapter{} }

func (a *AlertmanagerAdapter) Name() string { return "alertmanager" }

// alertmanagerPayload mirrors the Alertmanager webhook JSON structure.
type alertmanagerPayload struct {
	Status string             `json:"status"`
	Alerts []alertmanagerItem `json:"alerts"`
}

type alertmanagerItem struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

func (a *AlertmanagerAdapter) Adapt(payload []byte) ([]RawAlert, error) {
	var p alertmanagerPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}

	alerts := make([]RawAlert, 0, len(p.Alerts))
	for _, item := range p.Alerts {
		if item.Labels == nil {
			item.Labels = make(map[string]string)
		}
		item.Labels["source"] = "alertmanager"

		fp := item.Fingerprint
		if fp == "" {
			fp = ComputeFingerprint(item.Labels)
		}

		ra := RawAlert{
			Source:      "alertmanager",
			Fingerprint: fp,
			Labels:      item.Labels,
			Annotations: item.Annotations,
			StartsAt:    item.StartsAt,
			Status:      item.Status,
			RawPayload:  payload,
		}

		if !item.EndsAt.IsZero() {
			t := item.EndsAt
			ra.EndsAt = &t
		}

		alerts = append(alerts, ra)
	}

	return alerts, nil
}
