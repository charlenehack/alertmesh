package engine

import (
	"encoding/json"
	"regexp"

	"github.com/rs/zerolog/log"

	"github.com/kuzane/alertmesh/internal/ingestion"
)

// RouteDef defines a routing rule loaded from the database.
type RouteDef struct {
	ID       string
	Name     string
	Priority int
	Matchers []LabelMatcher
	GroupBy  []string
}

// LabelMatcher matches an alert label value by exact string or regex.
//
// Two on-disk JSON shapes are accepted (see UnmarshalJSON):
//
//   - Engine-native: {"name":"severity","value":"critical","isRegex":false}
//   - UI / Alertmanager-style: {"key":"severity","op":"=","value":"critical"}
//
// The UI form (web/src/pages/alert/AlertRoutes.tsx) writes the second shape
// because it reads more naturally to operators; the engine normalises both
// into the same in-memory representation.
type LabelMatcher struct {
	Name    string
	Value   string
	IsRegex bool
	Negate  bool
	// re is set exactly once by Router.SetRoutes during the
	// pre-compile pass.  Match()/Matches() read it without locking;
	// see the package-level race-safety note in Router below.
	re *regexp.Regexp
	// compileFailed is set to true if SetRoutes could not compile
	// `Value` as a regex.  Such a matcher is treated as "never
	// matches" so a malformed rule cannot accidentally widen the
	// route fan-out.
	compileFailed bool
}

// matcherJSON is the union of the two accepted on-disk shapes.
type matcherJSON struct {
	// Engine-native fields
	Name    string `json:"name,omitempty"`
	IsRegex bool   `json:"isRegex,omitempty"`

	// Alertmanager-style fields written by the UI
	Key string `json:"key,omitempty"`
	Op  string `json:"op,omitempty"`

	// Shared
	Value string `json:"value"`
}

// UnmarshalJSON accepts both engine-native and UI/Alertmanager-style shapes.
//
// Op semantics (UI form):
//
//	"="  exact match (default)
//	"!=" exact non-match (Negate=true)
//	"=~" regex match
//	"!~" regex non-match (Negate=true, IsRegex=true)
//
// Unknown ops fall back to exact match so a malformed UI payload still does
// the safest thing instead of silently matching everything.
func (m *LabelMatcher) UnmarshalJSON(data []byte) error {
	var raw matcherJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Name = raw.Name
	if m.Name == "" {
		m.Name = raw.Key
	}
	m.Value = raw.Value
	m.IsRegex = raw.IsRegex
	m.Negate = false

	switch raw.Op {
	case "", "=":
		// exact match (default)
	case "!=":
		m.Negate = true
	case "=~":
		m.IsRegex = true
	case "!~":
		m.IsRegex = true
		m.Negate = true
	}
	return nil
}

// Matches returns true if the label value satisfies this matcher.
//
// Concurrency: this is read-only on `m`.  All regex compilation happens
// once at SetRoutes time so multiple goroutines can call Matches in
// parallel without locking — critical now that the Kafka manager runs N
// independent worker goroutines per data source.
func (m *LabelMatcher) Matches(labels map[string]string) bool {
	v, ok := labels[m.Name]
	if !ok {
		// A non-existent label cannot match an exact-match rule, but a
		// negated rule against a missing label conventionally returns true.
		return m.Negate
	}
	hit := false
	if m.IsRegex {
		if m.compileFailed || m.re == nil {
			// Bad regex — never claims a match.  Negate doesn't flip
			// this; we'd rather drop a malformed rule on the floor
			// than let it accidentally swallow every alert.
			return false
		}
		hit = m.re.MatchString(v)
	} else {
		hit = v == m.Value
	}
	if m.Negate {
		return !hit
	}
	return hit
}

// Router holds a priority-ordered list of routing rules.
//
// Concurrency contract: SetRoutes is the only writer and is called from a
// single goroutine (the rule-engine reload loop).  Match()/matchesAll() are
// read-only — they never mutate the slice or any matcher's `re` field, so
// no lock is needed on the hot path.
type Router struct {
	routes []RouteDef
}

func NewRouter() *Router {
	return &Router{}
}

// SetRoutes replaces the current routing rules (called during init and hot-reload).
//
// We pre-compile every IsRegex matcher's `Value` here so the per-alert
// hot path stays read-only.  Compile failures are logged at warn level and
// the matcher is flagged so it permanently misses — operators see the
// problem in their logs without the engine wedging on a bad rule.
func (r *Router) SetRoutes(routes []RouteDef) {
	for ri := range routes {
		ms := routes[ri].Matchers
		for mi := range ms {
			m := &ms[mi]
			if !m.IsRegex {
				m.re = nil
				m.compileFailed = false
				continue
			}
			re, err := regexp.Compile(m.Value)
			if err != nil {
				log.Warn().
					Str("component", "router").
					Str("route_id", routes[ri].ID).
					Str("route_name", routes[ri].Name).
					Str("matcher_name", m.Name).
					Str("matcher_value", m.Value).
					Err(err).
					Msg("regex matcher failed to compile, route will never match this rule")
				m.re = nil
				m.compileFailed = true
				continue
			}
			m.re = re
			m.compileFailed = false
		}
	}
	r.routes = routes
}

// Match returns the first matching route for the alert, or nil if none match.
func (r *Router) Match(alert ingestion.RawAlert) *RouteDef {
	for i := range r.routes {
		route := &r.routes[i]
		if matchesAll(route.Matchers, alert.Labels) {
			return route
		}
	}
	return nil
}

func matchesAll(matchers []LabelMatcher, labels map[string]string) bool {
	for i := range matchers {
		if !matchers[i].Matches(labels) {
			return false
		}
	}
	return true
}
