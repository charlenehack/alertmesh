package realtime

import "encoding/json"

// Event types the backend emits over the realtime channel.  We
// intentionally keep the schema tiny: the frontend treats every event as
// "something about <incident_id> changed, refetch the relevant query".
//
// Adding richer fields later (e.g. a delta) would tempt the UI to render
// straight from the WS payload, which then has to stay in lock-step with
// the REST representation forever.  React-query invalidate-on-event keeps
// the REST endpoint as the single source of truth.
const (
	EventIncidentCreated  = "incident.created"
	EventIncidentUpdated  = "incident.updated"
	EventIncidentAppended = "incident.appended"
	EventIncidentAck      = "incident.ack"
	EventIncidentResolved = "incident.resolved"
	EventIncidentClosed   = "incident.closed"
)

// Event is the JSON envelope sent to subscribers.  Severity / Status are
// included so future UI affordances (e.g. a toast for new P0 events) can
// react without an extra REST round-trip, but they remain optional.
type Event struct {
	Type       string `json:"type"`
	IncidentID string `json:"incident_id,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Status     string `json:"status,omitempty"`
}

// Marshal returns the JSON-encoded form of the event suitable for
// pg_notify and for direct WebSocket fan-out.  Errors here are
// effectively impossible (the type only contains strings), so callers
// that ignore the error are fine; we return it for completeness.
func (e Event) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// TopicAll is the firehose topic — every incident lifecycle event is
// broadcast here, used by the incidents list and dashboard pages.
const TopicAll = "incidents"

// TopicIncident builds the per-incident topic name used by the incident
// detail page subscription.  Centralised so the encoding stays in one
// place if we ever change it (e.g. add a tenant prefix).
func TopicIncident(id string) string { return "incident:" + id }
