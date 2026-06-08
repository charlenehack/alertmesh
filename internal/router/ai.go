package router

import (
	"errors"
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	ai_pkg "github.com/kuzane/alertmesh/internal/ai"
	"github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/label"
	"github.com/kuzane/alertmesh/internal/model"
)

type aiHandler struct {
	db    *gorm.DB
	cfg   *config.Config
	wsHub *ai_pkg.WSHub
}

func newAIHandler(db *gorm.DB, wsHub *ai_pkg.WSHub, cfg *config.Config) *aiHandler {
	return &aiHandler{db: db, wsHub: wsHub, cfg: cfg}
}

func (h *aiHandler) registerRoutes(ws *restful.WebService) {
	ws.Route(ws.GET("/incidents/{id}/ai").
		To(h.getReport).
		Doc("Get AI analysis report").
		Metadata(label.MetaIdentity, label.IncidentAccess).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "AI").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/incidents/{id}/ai/trigger").
		To(h.trigger).
		Doc("Trigger AI analysis").
		Metadata(label.MetaIdentity, label.IncidentAccess).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "AI").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/incidents/{id}/ai/chat").
		To(h.chat).
		Doc("Follow-up AI conversation").
		Metadata(label.MetaIdentity, label.IncidentAccess).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "AI").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// WebSocket endpoint for streaming AI analysis steps.  JWT is
	// required (browsers can't set Authorization on `new WebSocket`,
	// so AuthFilter accepts `?token=` as a fallback — see middleware/
	// auth.go).  No ACL gate: any logged-in user who can read an
	// incident may also stream its AI events.  Without `auth: true`
	// metadata this route used to upgrade for completely anonymous
	// callers, which let anyone tail another tenant's analysis.
	ws.Route(ws.GET("/incidents/{id}/ai/ws").
		To(h.websocketHandler).
		Doc("WebSocket for AI analysis streaming. Pass `?token=<JWT>`.").
		Param(ws.QueryParameter("token", "JWT (browsers can't set Authorization on WebSocket)").Required(false)).
		Metadata(label.MetaAuth, label.Enable))

	// LLM provider management (admin-only configuration surface for the AI
	// backend). Lives under the AI handler so the routes are colocated with
	// the consumers that read llm_providers (Analyze / Chat).
	h.registerLLMProviderRoutes(ws)
}

func (h *aiHandler) getReport(req *restful.Request, resp *restful.Response) {
	incidentID := req.PathParameter("id")

	var analysis model.AIAnalysis
	if err := h.db.Where("incident_id = ?", incidentID).
		Order("created_at DESC").
		First(&analysis).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.Success(resp, map[string]any{
				"status":  "pending",
				"message": "No AI analysis report available yet",
			})
			return
		}
		httputil.InternalError(resp, err.Error())
		return
	}

	// Get conversation history
	var conversations []model.AIConversation
	h.db.Where("incident_id = ?", incidentID).
		Order("created_at ASC").
		Find(&conversations)

	httputil.Success(resp, map[string]any{
		"report":        analysis.Report,
		"summary":       analysis.Summary,
		"root_cause":    analysis.RootCause,
		"created_at":    analysis.CreatedAt,
		"conversations": conversations,
	})
}

func (h *aiHandler) trigger(req *restful.Request, resp *restful.Response) {
	incidentID := req.PathParameter("id")

	// Whitelist gate: only allow manual trigger when the originating data
	// source has ai_enabled=true.
	var inc model.Incident
	if err := h.db.WithContext(req.Request.Context()).
		Select("id", "ai_status", "data_source_id").
		Where("id = ?", incidentID).
		First(&inc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(resp)
			return
		}
		httputil.InternalError(resp, err.Error())
		return
	}

	dsID := ""
	if inc.DataSourceID != nil {
		dsID = *inc.DataSourceID
	}
	// dsID 为空说明该 incident 没有关联数据源（历史数据或外部源），允许手动触发
	// 如果有关联数据源，则该数据源必须开启 ai_enabled
	if dsID != "" && !ai_pkg.ShouldRun(req.Request.Context(), h.db, dsID) {
		httputil.BadRequest(resp, "该告警源未启用 AI 分析（请前往数据源配置开启 ai_enabled）")
		return
	}

	if err := ai_pkg.EnqueueTask(h.db, incidentID); err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, map[string]string{"status": "enqueued"})
}

type chatRequest struct {
	Message string `json:"message"`
}

func (h *aiHandler) chat(req *restful.Request, resp *restful.Response) {
	incidentID := req.PathParameter("id")
	userID, _ := req.Attribute("user_id").(string)

	var body chatRequest
	if err := req.ReadEntity(&body); err != nil || body.Message == "" {
		httputil.BadRequest(resp, "message is required")
		return
	}

	// Save user message
	memory := ai_pkg.NewMemory(h.db)
	if err := memory.AddMessage(req.Request.Context(), incidentID, "user", body.Message, userID); err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}

	// Get conversation history
	history, err := memory.GetHistory(req.Request.Context(), incidentID)
	if err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}

	// Create streaming callback for this incident
	cb := ai_pkg.NewStreamCallback(h.wsHub, incidentID)

	// Run conversational agent
	agent := ai_pkg.NewAgent(h.db, h.cfg)
	reply, err := agent.Chat(req.Request.Context(), incidentID, body.Message, history, cb)
	if err != nil {
		log.Error().Err(err).Str("incident_id", incidentID).Msg("AI chat error")
		httputil.InternalError(resp, "AI chat failed: "+err.Error())
		return
	}

	// Save assistant reply.  A persistence failure here only means the next
	// turn of the conversation won't see this reply in its memory window;
	// the user already has the reply in the HTTP response, so log + move on.
	if err := memory.AddMessage(req.Request.Context(), incidentID, "assistant", reply, ""); err != nil {
		log.Warn().Err(err).Str("incident_id", incidentID).Msg("failed to persist assistant reply")
	}

	httputil.Success(resp, map[string]string{
		"reply": reply,
	})
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *aiHandler) websocketHandler(req *restful.Request, resp *restful.Response) {
	incidentID := req.PathParameter("id")

	// Defensive: AuthFilter already enforces JWT for this route via the
	// `auth: true` metadata + `?token=` query fallback.  This second
	// check guards against the metadata being accidentally removed —
	// it's cheap and saves us from silently regressing to anonymous
	// AI streaming, which was the original bug.
	if uid, _ := req.Attribute("user_id").(string); uid == "" {
		httputil.Unauthorized(resp)
		return
	}

	conn, err := upgrader.Upgrade(resp.ResponseWriter, req.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("ws upgrade failed")
		return
	}

	h.wsHub.Register(incidentID, conn)
	defer func() {
		h.wsHub.Unregister(incidentID, conn)
		_ = conn.Close()
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
