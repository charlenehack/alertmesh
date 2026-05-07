package metrics

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Alert and incident counters exposed to ingestion and engine packages.
var (
	AlertsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_alerts_received_total",
			Help: "Total number of raw alerts received, partitioned by source.",
		},
		[]string{"source"},
	)

	IncidentsCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "alertmesh_incidents_created_total",
		Help: "Total number of incidents created by the rule engine.",
	})

	IncidentStatusChanges = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_incident_status_changes_total",
			Help: "Total number of incident status transitions, partitioned by to_status.",
		},
		[]string{"to_status"},
	)

	IncidentsEscalated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_incidents_escalated_total",
			Help: "Total number of incidents whose severity was bumped by an escalation policy.",
		},
		[]string{"from", "to"},
	)

	PipelineDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_pipeline_dropped_total",
			Help: "Alerts dropped by the rule-engine pipeline, partitioned by reason.",
		},
		[]string{"reason"},
	)

	HTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_http_requests_total",
			Help: "Total HTTP requests handled, partitioned by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmesh_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	AITasksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_ai_tasks_total",
			Help: "Total AI analysis tasks, partitioned by status (done|failed).",
		},
		[]string{"status"},
	)

	// NotificationsDropped counts incidents the dispatcher refused to deliver
	// because no AlertRoute / NotificationPolicy / contact resolved.  Reasons:
	//
	//   nil_incident          - DispatchForIncident called without an incident
	//   no_route              - incident did not match any AlertRoute (operator
	//                           must add a catch-all route with empty matchers)
	//   no_policy             - matched route has no policies attached
	//   policy_lookup_failed  - DB error when fetching the linked policies
	//   no_severity_match     - linked policies are configured but none cover
	//                           this incident's severity
	//   no_contacts           - matched policies resolve to zero contacts
	NotificationsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_notifications_dropped_total",
			Help: "Incidents the dispatcher refused to deliver, partitioned by reason.",
		},
		[]string{"reason"},
	)

	// IncidentsAutoResolved counts incidents the system closed without an
	// operator click.  reason ∈ {endsat_signal, staleness}:
	//   endsat_signal — Prometheus pushed an alert with endsAt ≤ now
	//                   (the upstream clean-recovery signal)
	//   staleness     — no firing alert seen within incident.staleness_timeout
	IncidentsAutoResolved = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_incidents_auto_resolved_total",
			Help: "Open incidents auto-closed by upstream resolved signal or the staleness reaper.",
		},
		[]string{"reason"},
	)

	// IncidentsReopened counts how often a same-group_key alert lands inside
	// the incident.reopen_window cool-down and flips a previously resolved
	// incident back to open instead of creating a fresh row.
	IncidentsReopened = promauto.NewCounter(prometheus.CounterOpts{
		Name: "alertmesh_incidents_reopened_total",
		Help: "Resolved incidents promoted back to open within the reopen window.",
	})

	// IncidentRepeatNotifications tracks every dispatch that came out of the
	// progressive repeat schedule, partitioned by the title-prefix tag of
	// the rung that fired it (e.g. [REPEAT], [ATTENTION], [CRITICAL]).
	// Useful for dashboards that want to surface escalation noise vs.
	// routine reminders.
	IncidentRepeatNotifications = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_incident_repeat_notifications_total",
			Help: "Re-notifications dispatched by the progressive repeat schedule, partitioned by rung tag.",
		},
		[]string{"tag"},
	)

	// IncidentRepeatSequenceStep observes the per-incident repeat_seq_index
	// at the moment of dispatch — i.e. "this was the Nth re-notification
	// at the current severity tier".  Useful for spotting incidents that
	// keep flapping around the dense head of the sequence vs. ones that
	// burn through the tail and run at IntervalMax forever.  The index
	// resets to 0 on every severity escalation.
	IncidentRepeatSequenceStep = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "alertmesh_incident_repeat_sequence_step",
		Help:    "Per-tier sequence index at the moment a repeat notification fires (0 = first repeat at this tier).",
		Buckets: []float64{0, 1, 2, 3, 5, 8, 13, 21, 34, 55},
	})

	// NotificationsDispatched counts every (channel, contact) tuple the
	// dispatcher actually fanned out to, partitioned by the channel type
	// and a `batched` boolean indicating whether the channel hit was a
	// batched merge (single API call covering ≥2 recipients) or a single-
	// recipient fan-out.  Lets dashboards surface IM/email merging as
	// concrete deliverable savings.
	NotificationsDispatched = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_notifications_dispatched_total",
			Help: "Notifications successfully dispatched, partitioned by channel and whether the call was batched.",
		},
		[]string{"channel", "batched"},
	)

	// KafkaMessagesReceived counts every Kafka message the consumer pulled
	// off a topic, before filter / mapping are applied.  Combined with
	// KafkaMessagesDropped this is the SLI for "is the consumer healthy?".
	KafkaMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_kafka_messages_received_total",
			Help: "Kafka messages pulled by the consumer, partitioned by data source name.",
		},
		[]string{"datasource"},
	)

	// KafkaMessagesDropped counts messages the consumer refused to forward
	// to the engine pipeline.  reason ∈ {filter_false, missing_alertname,
	// missing_severity, bad_json, filter_error}.  filter_false is the
	// expected steady-state for "drop noisy log lines"; the others all
	// signal bad config or unexpected payload shape.
	KafkaMessagesDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alertmesh_kafka_messages_dropped_total",
			Help: "Kafka messages dropped by filter / mapping, partitioned by datasource and drop reason.",
		},
		[]string{"datasource", "reason"},
	)

	// KafkaConsumerLag exposes Reader.Stats().Lag — the number of
	// messages between the consumer's last commit and the latest offset
	// on the partition.  We label by datasource + partition so an
	// uneven-partition cluster surfaces the hot shard clearly.
	KafkaConsumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alertmesh_kafka_consumer_lag",
			Help: "Per-partition consumer lag in messages.",
		},
		[]string{"datasource", "partition"},
	)

	// KafkaProcessLatency observes the wall time spent on filter +
	// mapping + engine.Pipeline.Process for a single Kafka message,
	// labelled by data source and outcome (ok / filter_false /
	// missing_alertname / ...).  This is the primary diagnostic for
	// "why is the consumer not keeping up": a healthy access-log
	// consumer sits in the sub-millisecond bucket; lock contention on
	// the engine pipeline shows up as a long right tail with no
	// network or external dependency to blame.  Buckets stretch up to
	// 5s so a wedged downstream is also visible without re-bucketing.
	KafkaProcessLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alertmesh_kafka_process_latency_seconds",
			Help:    "Per-message processing latency for Kafka consumer goroutines (filter + mapping + engine), labelled by datasource and outcome.",
			Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"datasource", "outcome"},
	)
)

// Handler returns the standard Prometheus /metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

type healthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// HealthHandler serves GET /healthz with a simple liveness response.
// No auth is required for this endpoint.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
	})
}
