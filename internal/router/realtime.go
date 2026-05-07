package router

import (
	"net/http"
	"regexp"
	"strings"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/label"
	"github.com/kuzane/alertmesh/internal/realtime"
)

// realtimeHandler exposes the topic-based push channel that supersedes
// every UI polling timer (incidents list 20s / dashboard 30s / incident
// detail 15s).  See package-level docs in internal/realtime for the
// pg_notify -> hub -> WebSocket data flow.
type realtimeHandler struct {
	hub *realtime.Hub
}

func newRealtimeHandler(hub *realtime.Hub) *realtimeHandler {
	return &realtimeHandler{hub: hub}
}

// maxTopicsPerSubscriber bounds the cardinality of one WebSocket
// connection's subscription set so a single browser tab can't pin
// arbitrary memory in the hub by passing thousands of topics.  20 is
// comfortably above the worst legitimate case (a power user with the
// list page + a handful of detail pages stitched into one tab).
const maxTopicsPerSubscriber = 20

// topicPattern enumerates the legal topic shapes.  Centralising this in
// one regex keeps the route handler from quietly accepting typos like
// `incident_42` and the hub from sprouting topics that no event ever
// targets.  UUID-v4-with-or-without-dashes — we accept the standard
// hyphenated form that the rest of the API uses.
var topicPattern = regexp.MustCompile(`^(?:incidents|incident:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// realtimeUpgrader is dedicated to this route so we can tune buffer
// sizes / origin checks independently of the AI streaming endpoint.
// CheckOrigin returns true for now because the JWT in `?token=` already
// proves the caller has logged in; tighten only if we ever serve the
// React app from a different origin than the API.
var realtimeUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(_ *http.Request) bool { return true },
}

func (h *realtimeHandler) registerRoutes(ws *restful.WebService) {
	// Note: no ACL gate here on purpose.  Authentication via JWT (via
	// AuthFilter's new `?token=` fallback) is sufficient — the topic
	// regex already ensures every subscription is either the global
	// firehose or a per-incident channel, both of which the user is
	// allowed to see if they can read /api/v1/incidents at all.  When
	// per-incident ACL lands at the REST layer, this is the place to
	// add a parallel check for the `incident:<uuid>` shape.
	ws.Route(ws.GET("/realtime/ws").
		To(h.subscribe).
		Doc("WebSocket push channel for incident lifecycle events. "+
			"Pass `?topics=incidents,incident:<id>&token=<JWT>`. "+
			"Replaces the deprecated polling on /incidents.").
		Param(ws.QueryParameter("topics", "comma-separated topic list").Required(true)).
		Param(ws.QueryParameter("token", "JWT (browsers can't set Authorization on WebSocket)").Required(false)).
		Metadata(label.MetaModule, "Realtime").
		Metadata(label.MetaKind, "Realtime").
		Metadata(label.MetaAuth, label.Enable))
}

func (h *realtimeHandler) subscribe(req *restful.Request, resp *restful.Response) {
	// AuthFilter has already populated user_id (or rejected with 401)
	// when MetaAuth is enabled; this defensive check guards against the
	// route being moved out of the auth-protected group by accident.
	if uid, _ := req.Attribute("user_id").(string); uid == "" {
		httputil.Unauthorized(resp)
		return
	}

	topics, err := parseTopics(req.QueryParameter("topics"))
	if err != nil {
		httputil.BadRequest(resp, err.Error())
		return
	}

	conn, err := realtimeUpgrader.Upgrade(resp.ResponseWriter, req.Request, nil)
	if err != nil {
		// Upgrade has already written a response on failure; nothing
		// useful we can do beyond logging.
		log.Debug().Err(err).Msg("realtime: ws upgrade failed")
		return
	}

	username, _ := req.Attribute("username").(string)
	log.Debug().
		Str("component", "realtime").
		Str("user", username).
		Strs("topics", topics).
		Msg("realtime subscriber connected")

	sub := h.hub.Subscribe(conn, topics)
	// Run blocks until either side closes, then unsubscribes via the
	// hub and returns.  The Subscriber owns conn lifecycle from here.
	sub.Run()
}

// parseTopics validates and de-duplicates the `topics` query parameter.
// We reject empty/oversized lists and any topic that doesn't match
// topicPattern so the hub never grows entries that nothing will ever
// target.  Returns a stable order so logs / metrics are deterministic.
func parseTopics(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errEmptyTopics
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxTopicsPerSubscriber {
		return nil, errTooManyTopics
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if !topicPattern.MatchString(t) {
			return nil, errInvalidTopic(t)
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, errEmptyTopics
	}
	return out, nil
}

// errEmptyTopics et al. are package-level sentinel errors so the handler
// can convert them to 400 responses without re-allocating strings on
// every bad request.

type subscribeError string

func (e subscribeError) Error() string { return string(e) }

const (
	errEmptyTopics   subscribeError = "topics query parameter is required (e.g. ?topics=incidents)"
	errTooManyTopics subscribeError = "too many topics (max 20 per connection)"
)

func errInvalidTopic(t string) error {
	return subscribeError("invalid topic " + t + " (allowed: incidents, incident:<uuid>)")
}
