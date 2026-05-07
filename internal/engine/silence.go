package engine

import (
	"time"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// SilenceRule defines a time-windowed silence that suppresses matching alerts.
type SilenceRule struct {
	Name     string
	Matchers []LabelMatcher
	StartsAt time.Time
	EndsAt   time.Time
}

// Silencer evaluates time-based silence policies against incoming alerts.
type Silencer struct {
	rules []SilenceRule
}

func NewSilencer() *Silencer {
	return &Silencer{}
}

// SetRules replaces the silence rules (called during init and hot-reload).
func (s *Silencer) SetRules(rules []SilenceRule) {
	s.rules = rules
}

// IsSilenced returns true if the alert matches any active silence policy.
func (s *Silencer) IsSilenced(alert ingestion.RawAlert) bool {
	now := time.Now()
	for _, rule := range s.rules {
		if now.Before(rule.StartsAt) || now.After(rule.EndsAt) {
			continue
		}
		if matchesAll(rule.Matchers, alert.Labels) {
			return true
		}
	}
	return false
}
