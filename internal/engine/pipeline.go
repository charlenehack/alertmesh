package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/ingestion"
	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/pkg/metrics"
)

// AlertGroup represents a set of alerts aggregated by a common group key.
//
// DataSourceID propagates the originating data_sources.id from the first
// alert in the group; incident.Service uses it for AI eligibility (ai_enabled,
// ai_auto_trigger).  Empty when the upstream
// adapter doesn't yet attach a source id (legacy webhook / alertmanager).
type AlertGroup struct {
	GroupKey     string
	Labels       map[string]string
	Alerts       []ingestion.RawAlert
	Severity     string
	RouteID      string
	RouteName    string
	DataSourceID string
}

// IncidentCallback is invoked when the engine determines an AlertGroup should create or update an Incident.
type IncidentCallback func(group AlertGroup)

// ResolvedCallback is invoked the moment a single resolved-status alert is
// observed (Prometheus signals resolution by setting endsAt ≤ now).  It is
// fired *outside* the dedup / aggregation / inhibition pipeline so the
// incident layer can immediately auto-close the matching open incident
// instead of waiting for the next group-flush window.
//
// groupKey is computed using the same Router → AggregationPolicy resolution
// the firing path uses, so the receiver can do an O(1) lookup against
// incidents.group_key without re-implementing the routing rules.
type ResolvedCallback func(alert ingestion.RawAlert, groupKey string)

// Pipeline orchestrates the rule engine stages: dedup -> silence -> routing -> aggregation -> inhibition -> callback.
type Pipeline struct {
	db         *gorm.DB
	dedup      *Deduplicator
	router     *Router
	aggregator *Aggregator
	inhibitor  *Inhibitor
	silencer   *Silencer
	onIncident IncidentCallback
	onResolved ResolvedCallback

	mu     sync.RWMutex
	cancel context.CancelFunc
}

func NewPipeline(db *gorm.DB, onIncident IncidentCallback, onResolved ResolvedCallback) *Pipeline {
	p := &Pipeline{
		db:         db,
		dedup:      NewDeduplicator(),
		router:     NewRouter(),
		aggregator: NewAggregator(),
		inhibitor:  NewInhibitor(),
		silencer:   NewSilencer(),
		onIncident: onIncident,
		onResolved: onResolved,
	}
	p.loadRulesFromDB()
	return p
}

// Inhibitor exposes the inhibitor for external use (mainly testing).
func (p *Pipeline) Inhibitor() *Inhibitor {
	return p.inhibitor
}

// Process feeds a normalised RawAlert through the engine stages.
func (p *Pipeline) Process(alert ingestion.RawAlert) {
	engineLog := log.With().Str("component", "engine").Logger()

	metrics.AlertsReceived.WithLabelValues(alert.Source).Inc()

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Resolved-status alerts (Prometheus pushed endsAt ≤ now via the v2
	// adapter) take a fast path: hand straight off to the incident layer
	// so it can auto-close the matching open incident without paying the
	// dedup / silence / aggregation latency.  Skipping dedup is correct
	// because the resolved signal is itself the de-duplicator — a second
	// resolved arriving for an already-closed incident is a cheap no-op
	// in HandleResolvedAlert (FindActive returns ErrRecordNotFound).
	if alert.Status == "resolved" {
		// Compute the same group_key the firing path would have produced so
		// the incident layer can lookup the open incident directly.
		route := p.router.Match(alert)
		groupBy, _ := p.aggregator.resolve(alert, route)
		groupKey := computeGroupKey(alert.Labels, groupBy)
		engineLog.Debug().
			Str("fingerprint", alert.Fingerprint).
			Str("source", alert.Source).
			Str("group_key", groupKey).
			Msg("resolved-status alert routed to onResolved")
		if p.onResolved != nil {
			p.onResolved(alert, groupKey)
		}
		return
	}

	// Track every received alert as a potential inhibit source before dedup so
	// that frequent re-firing critical alerts keep their target alerts inhibited.
	p.inhibitor.Track(alert)

	if p.dedup.IsDuplicate(alert) {
		// Include source / data_source_id so operators can grep
		// "duplicate alert dropped" + their kafka ds name and immediately
		// see the folding rate — without this, a kafka source whose
		// fingerprint mapping has tiny cardinality (e.g. just
		// route_name|normalized_path on a Higress access-log topic)
		// looks identical to "consumer not running" from the outside.
		engineLog.Debug().
			Str("fingerprint", alert.Fingerprint).
			Str("source", alert.Source).
			Str("data_source_id", alert.DataSourceID).
			Msg("duplicate alert dropped")
		metrics.PipelineDropped.WithLabelValues("dedup").Inc()
		return
	}

	if p.silencer.IsSilenced(alert) {
		engineLog.Debug().
			Str("fingerprint", alert.Fingerprint).
			Msg("alert silenced")
		metrics.PipelineDropped.WithLabelValues("silence").Inc()
		return
	}

	route := p.router.Match(alert)
	if route == nil {
		engineLog.Warn().
			Str("fingerprint", alert.Fingerprint).
			Msg("no matching route, using default")
	}

	p.aggregator.Add(alert, route, func(group AlertGroup) {
		if p.inhibitor.IsInhibited(group) {
			engineLog.Debug().
				Str("group_key", group.GroupKey).
				Msg("alert group inhibited")
			metrics.PipelineDropped.WithLabelValues("inhibit").Inc()
			return
		}

		if route != nil {
			group.RouteID = route.ID
			group.RouteName = route.Name
		}

		if p.onIncident != nil {
			p.onIncident(group)
		}
	})
}

// Reload reloads rules from the database (called on hot-reload signal).
func (p *Pipeline) Reload() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loadRulesFromDB()
	log.Info().Msg("rule engine reloaded")
}

// StartReloadListener subscribes to PG NOTIFY 'pipeline_reload' channel and
// triggers Reload() whenever a notification arrives.  It blocks until ctx is
// cancelled, automatically reconnecting on error.  Run it in a goroutine.
func (p *Pipeline) StartReloadListener(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := p.listenReload(ctx); err != nil {
				log.Warn().Err(err).Msg("pipeline reload listener error, will retry")
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}()
}

// Stop terminates the reload listener.
func (p *Pipeline) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
}

// listenReload owns one long-lived PostgreSQL connection that issues
// `LISTEN pipeline_reload` and then blocks on libpq-style asynchronous
// notifications.  This is event-driven, NOT polling: the connection only
// wakes up when a backend issues `NOTIFY` (e.g. via the
// `internal/router/alert_center.go::notifyPipeline` helper after a
// route/silence/aggregation/inhibit/escalation CRUD).
//
// Implementation note: the project uses pgx v5 via gorm's postgres driver.
// `database/sql` exposes the underlying connection as `*stdlib.Conn`; the
// real LISTEN/NOTIFY API lives on the pgx connection it wraps, which we
// reach through the stdlib.Conn.Conn() accessor.  An earlier version of
// this code asserted against the lib/pq interface signature (which never
// matched pgx), fell through to a `time.Sleep(5 * time.Second); return nil`
// fallback, and treated the nil error as "notification arrived" — silently
// degrading the listener into a 5-second polling loop that called
// p.Reload() forever.  Don't reintroduce that pattern.
func (p *Pipeline) listenReload(ctx context.Context) error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "LISTEN pipeline_reload"); err != nil {
		return err
	}
	log.Info().Msg("pipeline reload listener started")

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		var notif *pgconn.Notification
		err := conn.Raw(func(driverConn any) error {
			// pgx's stdlib wrapper exposes the underlying *pgx.Conn via
			// .Conn(); that's where WaitForNotification lives.  If the
			// driver is something else (older lib/pq, future swap-out, …)
			// we surface a hard error so the outer goroutine reconnects
			// rather than silently degrading to polling.
			type pgxConner interface{ Conn() *pgx.Conn }
			pc, ok := driverConn.(pgxConner)
			if !ok {
				return fmt.Errorf("unsupported db driver conn %T: pipeline reload requires pgx", driverConn)
			}

			// Cap the wait so process shutdown / context cancellation
			// can unblock us within a bounded window.  The deadline is
			// the *only* timer in this loop — when it fires we just
			// keep waiting, we do NOT reload.
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			n, werr := pc.Conn().WaitForNotification(waitCtx)
			if werr != nil {
				return werr
			}
			notif = n
			return nil
		})

		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Idle timeout is the expected steady state — go back to waiting
		// without touching the rule engine.
		if errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		if err != nil {
			// Hard error on the connection (closed by server, network
			// blip, …): bubble up so StartReloadListener reconnects.
			return err
		}

		payload := ""
		if notif != nil {
			payload = notif.Payload
		}
		log.Debug().Str("payload", payload).Msg("pipeline_reload notification received")
		p.Reload()
	}
}

func (p *Pipeline) loadRulesFromDB() {
	p.loadRoutes()
	p.loadSilences()
	p.loadInhibits()
	p.loadAggregations()
}

func (p *Pipeline) loadRoutes() {
	var routes []model.AlertRoute
	if err := p.db.Where("is_enabled = ?", true).Order("priority DESC").Find(&routes).Error; err != nil {
		log.Error().Err(err).Msg("failed to load alert routes from DB")
		return
	}

	defs := make([]RouteDef, 0, len(routes))
	for _, r := range routes {
		var matchers []LabelMatcher
		if err := json.Unmarshal(r.Matchers, &matchers); err != nil {
			log.Warn().Err(err).Str("route", r.Name).Msg("invalid matchers JSON, skipping route")
			continue
		}
		var groupBy []string
		if len(r.GroupBy) > 0 {
			if err := json.Unmarshal(r.GroupBy, &groupBy); err != nil {
				log.Warn().Err(err).Str("route", r.Name).Msg("invalid group_by JSON, ignoring")
			}
		}
		defs = append(defs, RouteDef{
			ID:       r.ID,
			Name:     r.Name,
			Priority: r.Priority,
			Matchers: matchers,
			GroupBy:  groupBy,
		})
	}

	p.router.SetRoutes(defs)
	log.Debug().Int("count", len(defs)).Msg("alert routes loaded from DB")
}

func (p *Pipeline) loadInhibits() {
	var rows []model.InhibitRule
	if err := p.db.Where("is_enabled = ?", true).Find(&rows).Error; err != nil {
		log.Error().Err(err).Msg("failed to load inhibit rules from DB")
		return
	}

	rules := make([]InhibitRule, 0, len(rows))
	for _, r := range rows {
		var src, tgt []LabelMatcher
		if err := json.Unmarshal(r.SourceMatchers, &src); err != nil {
			log.Warn().Err(err).Str("rule", r.Name).Msg("invalid source_matchers JSON, skipping inhibit rule")
			continue
		}
		if err := json.Unmarshal(r.TargetMatchers, &tgt); err != nil {
			log.Warn().Err(err).Str("rule", r.Name).Msg("invalid target_matchers JSON, skipping inhibit rule")
			continue
		}
		var equal []string
		if len(r.Equal) > 0 {
			_ = json.Unmarshal(r.Equal, &equal)
		}
		rules = append(rules, InhibitRule{
			Name:           r.Name,
			SourceMatchers: src,
			TargetMatchers: tgt,
			Equal:          equal,
		})
	}

	p.inhibitor.SetRules(rules)
	log.Debug().Int("count", len(rules)).Msg("inhibit rules loaded from DB")
}

func (p *Pipeline) loadAggregations() {
	var rows []model.AggregationPolicy
	if err := p.db.Where("is_enabled = ?", true).Find(&rows).Error; err != nil {
		log.Error().Err(err).Msg("failed to load aggregation policies from DB")
		return
	}

	policies := make([]AggPolicyDef, 0, len(rows))
	for _, r := range rows {
		var matchers []LabelMatcher
		if err := json.Unmarshal(r.Matchers, &matchers); err != nil {
			log.Warn().Err(err).Str("policy", r.Name).Msg("invalid matchers JSON, skipping aggregation policy")
			continue
		}
		var groupBy []string
		if len(r.GroupBy) > 0 {
			_ = json.Unmarshal(r.GroupBy, &groupBy)
		}
		policies = append(policies, AggPolicyDef{
			Name:      r.Name,
			Matchers:  matchers,
			GroupBy:   groupBy,
			GroupWait: time.Duration(r.GroupWait) * time.Second,
		})
	}

	p.aggregator.SetPolicies(policies)
	log.Debug().Int("count", len(policies)).Msg("aggregation policies loaded from DB")
}

func (p *Pipeline) loadSilences() {
	var silences []model.SilencePolicy
	now := time.Now()
	if err := p.db.Where("is_active = ? AND starts_at <= ? AND ends_at >= ?", true, now, now).
		Find(&silences).Error; err != nil {
		log.Error().Err(err).Msg("failed to load silence policies from DB")
		return
	}

	rules := make([]SilenceRule, 0, len(silences))
	for _, s := range silences {
		var matchers []LabelMatcher
		if err := json.Unmarshal(s.Matchers, &matchers); err != nil {
			log.Warn().Err(err).Str("silence", s.Name).Msg("invalid matchers JSON, skipping silence")
			continue
		}
		rules = append(rules, SilenceRule{
			Name:     s.Name,
			Matchers: matchers,
			StartsAt: s.StartsAt,
			EndsAt:   s.EndsAt,
		})
	}

	p.silencer.SetRules(rules)
	log.Debug().Int("count", len(rules)).Msg("silence policies loaded from DB")
}
