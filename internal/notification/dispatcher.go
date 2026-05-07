package notification

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/pkg/metrics"
)

// Message holds the notification content to send.
type Message struct {
	IncidentID string
	Title      string
	Severity   string
	Body       string
	URL        string // link to the incident in the web UI
}

// Dispatcher routes notifications to configured channels and contacts.
type Dispatcher struct {
	db            *gorm.DB
	channels      map[string]Channel
	encryptionKey string
}

// NewDispatcher creates a Dispatcher.  encryptionKey is the AES-256 master
// key used to decrypt contact secrets and the system SMTP config; it may be
// empty in which case secret fields are assumed to be plain text.
func NewDispatcher(db *gorm.DB, encryptionKey string) *Dispatcher {
	d := &Dispatcher{
		db:            db,
		channels:      make(map[string]Channel),
		encryptionKey: encryptionKey,
	}
	d.channels["dingtalk"] = &DingTalkChannel{}
	d.channels["feishu"] = &FeishuChannel{}
	d.channels["slack"] = &SlackChannel{}
	d.channels["email"] = &EmailChannel{}
	d.channels["voice"] = NewVoiceChannel(nil)
	d.channels["sms"] = NewSMSChannel(nil)
	return d
}

// DispatchForIncident routes a notification using the policy/contact graph.
//
// route.ChannelIDs is interpreted as a JSON array of NotificationPolicy IDs.
// Each enabled policy whose severities contain msg.Severity contributes its
// contact_ids and group_ids; the union of contacts (deduplicated) is then
// notified across every channel they have configured.
//
// There is no longer a fallback path: if any stage produces zero results we
// log an ERROR and bump alertmesh_notifications_dropped_total{reason=…}.
// Operators MUST configure a catch-all AlertRoute (matchers=[],
// priority=0) wired to a SRE-on-call NotificationPolicy if they want to be
// sure no incident is silently swallowed.
func (d *Dispatcher) DispatchForIncident(
	ctx context.Context, msg Message, incident *model.Incident, encryptionKey string,
) {
	if encryptionKey == "" {
		encryptionKey = d.encryptionKey
	}

	if incident == nil {
		d.dropIncident(msg, "", "nil_incident",
			"dispatcher: nil incident passed to DispatchForIncident")
		return
	}

	route := d.loadRouteForIncident(ctx, incident)
	if route == nil {
		d.dropIncident(msg, "", "no_route",
			"dispatcher: incident matched no AlertRoute — please configure a catch-all AlertRoute with empty matchers")
		return
	}

	policyIDs := channelIDsToPolicyIDs(route.ChannelIDs)
	if len(policyIDs) == 0 {
		d.dropIncident(msg, route.Name, "no_policy",
			"dispatcher: matched AlertRoute has no notification policies attached")
		return
	}

	var policies []model.NotificationPolicy
	if err := d.db.WithContext(ctx).
		Where("id IN ? AND is_enabled = ?", policyIDs, true).
		Find(&policies).Error; err != nil {
		log.Error().Err(err).
			Str("incident_id", msg.IncidentID).
			Str("route", route.Name).
			Msg("dispatcher: failed to load notification policies, dropping incident")
		metrics.NotificationsDropped.WithLabelValues("policy_lookup_failed").Inc()
		return
	}

	matched := make([]model.NotificationPolicy, 0, len(policies))
	for _, p := range policies {
		if severityMatches(p.Severities, msg.Severity) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		d.dropIncident(msg, route.Name, "no_severity_match",
			"dispatcher: linked notification policies do not cover this incident's severity")
		return
	}

	var contactIDs, groupIDs []string
	for _, p := range matched {
		var c, g []string
		if len(p.ContactIDs) > 0 {
			_ = json.Unmarshal(p.ContactIDs, &c)
			contactIDs = append(contactIDs, c...)
		}
		if len(p.GroupIDs) > 0 {
			_ = json.Unmarshal(p.GroupIDs, &g)
			groupIDs = append(groupIDs, g...)
		}
	}

	contacts := d.loadAndDecryptContacts(ctx, contactIDs, groupIDs, encryptionKey)
	if len(contacts) == 0 {
		log.Error().
			Str("incident_id", msg.IncidentID).
			Str("severity", msg.Severity).
			Str("route", route.Name).
			Str("policies", joinPolicyIDs(matched)).
			Msg("dispatcher: matched policies resolve to zero contacts, dropping incident")
		metrics.NotificationsDropped.WithLabelValues("no_contacts").Inc()
		return
	}

	log.Info().
		Str("incident_id", msg.IncidentID).
		Str("severity", msg.Severity).
		Int("contacts", len(contacts)).
		Str("policies", joinPolicyIDs(matched)).
		Msg("dispatcher: notifying contacts")

	d.dispatchToContacts(ctx, msg, contacts)
}

// dropIncident records a structured ERROR + Prometheus counter increment for
// every incident the dispatcher refuses to deliver.  The legacy
// notification_channels fan-out fallback is intentionally gone: silent drops
// were the entire reason this refactor exists.
func (d *Dispatcher) dropIncident(msg Message, route, reason, why string) {
	log.Error().
		Str("incident_id", msg.IncidentID).
		Str("severity", msg.Severity).
		Str("route", route).
		Str("reason", reason).
		Msg(why)
	metrics.NotificationsDropped.WithLabelValues(reason).Inc()
}

// loadRouteForIncident returns the AlertRoute that originally matched this
// incident.  Lookup is by incident.route_id (set by the engine when the
// matching pipeline stage stamps it onto the AlertGroup); returns nil when
// no candidate is found.
func (d *Dispatcher) loadRouteForIncident(ctx context.Context, inc *model.Incident) *model.AlertRoute {
	if inc.RouteID == nil || *inc.RouteID == "" {
		return nil
	}
	var route model.AlertRoute
	if err := d.db.WithContext(ctx).
		Where("id = ?", *inc.RouteID).
		First(&route).Error; err != nil {
		if !gormErrorIsNoRow(err) {
			log.Warn().Err(err).Str("route_id", *inc.RouteID).Msg("dispatcher: lookup route by id failed")
		}
		return nil
	}
	return &route
}

func (d *Dispatcher) logNotification(ctx context.Context, incidentID, channelID, channelType, status, errMsg string) {
	entry := &model.NotificationLog{
		IncidentID:  incidentID,
		ChannelID:   channelID,
		ChannelType: channelType,
		Status:      status,
		Error:       errMsg,
	}
	d.db.WithContext(ctx).Create(entry)
}
