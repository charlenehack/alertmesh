package engine

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

const defaultInhibitSourceTTL = 10 * time.Minute

// InhibitRule defines a rule that silences alerts matching target labels when
// a source alert with matching labels is currently active and (optionally)
// shares the same value on every label listed in Equal.
type InhibitRule struct {
	Name           string
	SourceMatchers []LabelMatcher
	TargetMatchers []LabelMatcher
	Equal          []string
}

type sourceEntry struct {
	labels    map[string]string
	expiresAt time.Time
}

// Inhibitor evaluates inhibition rules against alert groups.  It keeps a
// short-TTL cache of recently seen alerts whose labels match any rule's source
// matcher; an alert group is inhibited when both target matchers fire and an
// active source still exists with equal label values.
//
// Concurrency model:
//
//   - rulesPtr is an atomic.Pointer so Track / IsInhibited can read the
//     current rules slice without acquiring a lock.  Production
//     deployments commonly run with zero inhibit rules — under the old
//     `mu sync.Mutex` Track() still entered the lock just to do the
//     `len(rules) == 0` check, which serialised the entire Kafka
//     consumer fan-out on every message.  Reading the pointer is one
//     atomic load on the hot path.
//   - sourcesMu protects the sources map.  Only entered when there's
//     actually at least one rule to potentially write into it
//     (Track-side) or to scan it from (IsInhibited-side).
//
// Hot-reload (SetRules) writes a brand-new slice and CAS-swaps the
// pointer — readers either see the old slice for one more call or the
// new one; both are valid (rules at the time of evaluation).
type Inhibitor struct {
	rulesPtr atomic.Pointer[[]InhibitRule]

	sourcesMu sync.Mutex
	sources   map[string]*sourceEntry

	ttl  time.Duration
	once sync.Once
}

func NewInhibitor() *Inhibitor {
	i := &Inhibitor{
		sources: make(map[string]*sourceEntry),
		ttl:     defaultInhibitSourceTTL,
	}
	empty := []InhibitRule{}
	i.rulesPtr.Store(&empty)
	return i
}

// SetRules replaces the inhibition rules (called during init and hot-reload).
func (i *Inhibitor) SetRules(rules []InhibitRule) {
	// Defensive copy so callers can mutate their slice afterwards
	// without racing the goroutines reading via rulesPtr.Load().
	cp := append([]InhibitRule(nil), rules...)
	i.rulesPtr.Store(&cp)
	i.once.Do(func() { go i.cleanup() })
}

// loadRules returns the current rules slice; nil/empty when no rules
// configured.  Callers must treat the returned slice as read-only.
func (i *Inhibitor) loadRules() []InhibitRule {
	p := i.rulesPtr.Load()
	if p == nil {
		return nil
	}
	return *p
}

// Track records an incoming alert as a potential inhibit source.  Should be
// called by the pipeline before dedup so high-frequency critical alerts
// continue to suppress derived warnings.
//
// Hot-path optimisation: when no rules are configured (the common case
// in production), we return after a single atomic load with no locking
// at all.  Previously this took a full Mutex on every alert which was
// the dominant serialisation point for the Kafka consumer fan-out.
func (i *Inhibitor) Track(alert ingestion.RawAlert) {
	rules := i.loadRules()
	if len(rules) == 0 {
		return
	}
	for _, r := range rules {
		if matchesAll(r.SourceMatchers, alert.Labels) {
			labelsCopy := make(map[string]string, len(alert.Labels))
			for k, v := range alert.Labels {
				labelsCopy[k] = v
			}
			entry := &sourceEntry{
				labels:    labelsCopy,
				expiresAt: time.Now().Add(i.ttl),
			}
			i.sourcesMu.Lock()
			i.sources[alert.Fingerprint] = entry
			i.sourcesMu.Unlock()
			return
		}
	}
}

// IsInhibited returns true when at least one rule fires:
//   - the group's labels satisfy TargetMatchers, AND
//   - there exists an active source whose labels satisfy SourceMatchers
//     and whose values for every key in Equal match the group's values.
func (i *Inhibitor) IsInhibited(group AlertGroup) bool {
	rules := i.loadRules()
	if len(rules) == 0 {
		return false
	}

	i.sourcesMu.Lock()
	defer i.sourcesMu.Unlock()

	now := time.Now()
	for _, rule := range rules {
		if !matchesAll(rule.TargetMatchers, group.Labels) {
			continue
		}
		for _, src := range i.sources {
			if now.After(src.expiresAt) {
				continue
			}
			if !matchesAll(rule.SourceMatchers, src.labels) {
				continue
			}
			if equalLabels(rule.Equal, src.labels, group.Labels) {
				return true
			}
		}
	}
	return false
}

// equalLabels returns true when every key in equal has the same value in both
// label maps (an empty equal list means "no equality constraint" → true).
func equalLabels(equal []string, a, b map[string]string) bool {
	for _, k := range equal {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

func (i *Inhibitor) cleanup() {
	ticker := time.NewTicker(i.ttl)
	defer ticker.Stop()
	for range ticker.C {
		i.sourcesMu.Lock()
		now := time.Now()
		for k, v := range i.sources {
			if now.After(v.expiresAt) {
				delete(i.sources, k)
			}
		}
		i.sourcesMu.Unlock()
	}
}
