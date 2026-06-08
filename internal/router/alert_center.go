package router

import (
	"context"
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/auth"
	cfgcrypto "github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/label"
	"github.com/kuzane/alertmesh/internal/model"
)

// emptyJSONArray is the JSON literal used to satisfy NOT NULL constraints on
// jsonb columns when the request payload omits the field.  The DB schema
// declares DEFAULT '[]' but PostgreSQL only applies the default when the
// column is omitted from the INSERT statement; GORM however always emits an
// explicit value (NULL when the Go field is nil), so we have to normalise
// nil → []  before persisting.
var emptyJSONArray = datatypes.JSON([]byte("[]"))

// ensureJSONArray returns v unchanged when it carries any payload, otherwise
// the canonical empty-array literal so jsonb NOT NULL columns stay happy.
func ensureJSONArray(v datatypes.JSON) datatypes.JSON {
	if len(v) == 0 {
		return emptyJSONArray
	}
	return v
}

// decodeContactClientCiphers RSA-decrypts every wire-encrypted secret field
// in-place using the shared auth.DecodeClientCipher helper.  Must be called
// BEFORE encryptContactSecrets / DB write.
func decodeContactClientCiphers(c *model.NotificationContact) {
	c.WebhookToken = auth.DecodeClientCipher(c.WebhookToken)
	c.SlackBotToken = auth.DecodeClientCipher(c.SlackBotToken)
	c.FeishuSecret = auth.DecodeClientCipher(c.FeishuSecret)
	c.DingtalkSecret = auth.DecodeClientCipher(c.DingtalkSecret)
}

type alertCenterHandler struct {
	db            *gorm.DB
	encryptionKey string // base64 AES-256 master key for secret fields
}

func newAlertCenterHandler(db *gorm.DB, encryptionKey string) *alertCenterHandler {
	return &alertCenterHandler{db: db, encryptionKey: encryptionKey}
}

// notifyPipeline broadcasts a pipeline_reload notification so the running
// engine refreshes routes/silences/aggregations/inhibits/escalations from the
// database without requiring a restart.
func (h *alertCenterHandler) notifyPipeline(ctx context.Context, kind string) {
	if err := h.db.WithContext(ctx).
		Exec("SELECT pg_notify('pipeline_reload', ?)", kind).Error; err != nil {
		log.Warn().Err(err).Str("kind", kind).Msg("failed to notify pipeline_reload")
	}
}

// encryptContactSecrets encrypts sensitive fields in-place before DB write.
func (h *alertCenterHandler) encryptContactSecrets(c *model.NotificationContact) {
	encrypt := func(val string) string {
		if val == "" {
			return ""
		}
		enc, err := cfgcrypto.Encrypt(val, h.encryptionKey)
		if err != nil {
			log.Warn().Err(err).Msg("failed to encrypt contact secret field")
			return val
		}
		return enc
	}
	c.WebhookToken = encrypt(c.WebhookToken)
	c.SlackBotToken = encrypt(c.SlackBotToken)
	c.FeishuSecret = encrypt(c.FeishuSecret)
	c.DingtalkSecret = encrypt(c.DingtalkSecret)
}

// maskSecret returns "******" if non-empty, used for list views.
func maskSecret(val string) string {
	if val == "" {
		return ""
	}
	return "******"
}

// encryptField encrypts a single field value, returning empty string for empty input.
func (h *alertCenterHandler) encryptField(val string) string {
	if val == "" {
		return ""
	}
	enc, err := cfgcrypto.Encrypt(val, h.encryptionKey)
	if err != nil {
		log.Warn().Err(err).Msg("failed to encrypt field")
		return val
	}
	return enc
}

func (h *alertCenterHandler) registerRoutes(ws *restful.WebService) {
	// ── Alert Routes ──────────────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/routes").To(h.listRoutes).Doc("List alert routes").
		Metadata(label.MetaIdentity, label.AlertRouteAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "AlertRoute").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/routes").To(h.createRoute).Doc("Create alert route").
		Metadata(label.MetaIdentity, label.AlertRouteAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "AlertRoute").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/routes/{id}").To(h.updateRoute).Doc("Update alert route").
		Metadata(label.MetaIdentity, label.AlertRouteAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "AlertRoute").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/routes/{id}").To(h.deleteRoute).Doc("Delete alert route").
		Metadata(label.MetaIdentity, label.AlertRouteAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "AlertRoute").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Notification Templates ────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/templates").To(h.listTemplates).Doc("List notification templates").
		Metadata(label.MetaIdentity, label.TemplateAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Template").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/templates").To(h.createTemplate).Doc("Create notification template").
		Metadata(label.MetaIdentity, label.TemplateAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Template").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/templates/{id}").To(h.updateTemplate).Doc("Update notification template").
		Metadata(label.MetaIdentity, label.TemplateAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Template").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/templates/{id}").To(h.deleteTemplate).Doc("Delete notification template").
		Metadata(label.MetaIdentity, label.TemplateAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Template").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Aggregation Policies ──────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/aggregations").To(h.listAggregations).Doc("List aggregation policies").
		Metadata(label.MetaIdentity, label.AggregationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Aggregation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/aggregations").To(h.createAggregation).Doc("Create aggregation policy").
		Metadata(label.MetaIdentity, label.AggregationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Aggregation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/aggregations/{id}").To(h.updateAggregation).Doc("Update aggregation policy").
		Metadata(label.MetaIdentity, label.AggregationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Aggregation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/aggregations/{id}").To(h.deleteAggregation).Doc("Delete aggregation policy").
		Metadata(label.MetaIdentity, label.AggregationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Aggregation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Silence Policies ──────────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/silences").To(h.listSilences).Doc("List silence policies").
		Metadata(label.MetaIdentity, label.SilenceAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Silence").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/silences").To(h.createSilence).Doc("Create silence policy").
		Metadata(label.MetaIdentity, label.SilenceAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Silence").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/silences/{id}").To(h.deleteSilence).Doc("Delete silence policy").
		Metadata(label.MetaIdentity, label.SilenceAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Silence").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Notification Policies ─────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/policies").To(h.listPolicies).Doc("List notification policies").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Policy").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/policies").To(h.createPolicy).Doc("Create notification policy").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Policy").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/policies/{id}").To(h.updatePolicy).Doc("Update notification policy").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Policy").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/policies/{id}").To(h.deletePolicy).Doc("Delete notification policy").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Policy").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Notification Contacts ─────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/contacts").To(h.listContacts).Doc("List contacts").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Contact").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/contacts").To(h.createContact).Doc("Create contact").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Contact").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/contacts/{id}").To(h.updateContact).Doc("Update contact").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Contact").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/contacts/{id}").To(h.deleteContact).Doc("Delete contact").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Contact").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Notification Contact Groups ───────────────────────────────────────────
	ws.Route(ws.GET("/alert/contact-groups").To(h.listContactGroups).Doc("List contact groups").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "ContactGroup").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/contact-groups").To(h.createContactGroup).Doc("Create contact group").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "ContactGroup").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/contact-groups/{id}").To(h.updateContactGroup).Doc("Update contact group").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "ContactGroup").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/contact-groups/{id}").To(h.deleteContactGroup).Doc("Delete contact group").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "ContactGroup").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Inhibit Rules ─────────────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/inhibits").To(h.listInhibits).Doc("List inhibit rules").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Inhibit").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/inhibits").To(h.createInhibit).Doc("Create inhibit rule").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Inhibit").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/inhibits/{id}").To(h.updateInhibit).Doc("Update inhibit rule").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Inhibit").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/inhibits/{id}").To(h.deleteInhibit).Doc("Delete inhibit rule").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Inhibit").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Escalation Policies ───────────────────────────────────────────────────
	ws.Route(ws.GET("/alert/escalations").To(h.listEscalations).Doc("List escalation policies").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Escalation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/alert/escalations").To(h.createEscalation).Doc("Create escalation policy").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Escalation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/alert/escalations/{id}").To(h.updateEscalation).Doc("Update escalation policy").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Escalation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.DELETE("/alert/escalations/{id}").To(h.deleteEscalation).Doc("Delete escalation policy").
		Metadata(label.MetaIdentity, label.NotificationAccess).
		Metadata(label.MetaModule, label.AlertCenterModuleName).
		Metadata(label.MetaKind, "Escalation").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// ── Webhook Sources (RFC 9421 trusted-source keyring) ─────────────────────
	h.registerWebhookSourceRoutes(ws)
}

// ─── Alert Routes ──────────────────────────────────────────────────────────────

func (h *alertCenterHandler) listRoutes(req *restful.Request, resp *restful.Response) {
	var rows []model.AlertRoute
	h.db.WithContext(req.Request.Context()).Order("priority desc").Find(&rows)
	httputil.Success(resp, rows)
}

// normalizeRoute fills in safe defaults so the row satisfies NOT NULL
// constraints when the UI omits the optional advanced fields.
//   - matchers / group_by / channel_ids default to empty JSON arrays.
//
// An empty matchers array means "match everything"; combined with the lowest
// priority this is the canonical shape of a catch-all (兜底) route.
func normalizeRoute(r *model.AlertRoute) {
	r.Matchers = ensureJSONArray(r.Matchers)
	r.GroupBy = ensureJSONArray(r.GroupBy)
	r.ChannelIDs = ensureJSONArray(r.ChannelIDs)
}

func (h *alertCenterHandler) createRoute(req *restful.Request, resp *restful.Response) {
	var row model.AlertRoute
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	normalizeRoute(&row)
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "route:create")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateRoute(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.AlertRoute
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	normalizeRoute(&row)
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "route:update")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteRoute(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.AlertRoute{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "route:delete")
	httputil.Success(resp, nil)
}

// ─── Notification Templates ────────────────────────────────────────────────────

func (h *alertCenterHandler) listTemplates(req *restful.Request, resp *restful.Response) {
	var rows []model.NotificationTemplate
	h.db.WithContext(req.Request.Context()).Find(&rows)
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createTemplate(req *restful.Request, resp *restful.Response) {
	var row model.NotificationTemplate
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateTemplate(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.NotificationTemplate
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteTemplate(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.NotificationTemplate{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, nil)
}

// ─── Aggregation Policies ──────────────────────────────────────────────────────

func (h *alertCenterHandler) listAggregations(req *restful.Request, resp *restful.Response) {
	var rows []model.AggregationPolicy
	h.db.WithContext(req.Request.Context()).Find(&rows)
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createAggregation(req *restful.Request, resp *restful.Response) {
	var row model.AggregationPolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "aggregation:create")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateAggregation(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.AggregationPolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "aggregation:update")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteAggregation(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.AggregationPolicy{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "aggregation:delete")
	httputil.Success(resp, nil)
}

// ─── Silence Policies ──────────────────────────────────────────────────────────

func (h *alertCenterHandler) listSilences(req *restful.Request, resp *restful.Response) {
	var rows []model.SilencePolicy
	h.db.WithContext(req.Request.Context()).Order("created_at desc").Find(&rows)
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createSilence(req *restful.Request, resp *restful.Response) {
	var row model.SilencePolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	username, _ := req.Attribute("username").(string)
	if username != "" {
		row.CreatedBy = username
	}
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "silence:create")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteSilence(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	result := h.db.WithContext(req.Request.Context()).
		Model(&model.SilencePolicy{}).
		Where("id = ?", id).
		Update("is_active", false)
	if result.Error != nil {
		httputil.InternalError(resp, result.Error.Error())
		return
	}
	if result.RowsAffected == 0 {
		httputil.Error(resp, http.StatusNotFound, "silence policy not found")
		return
	}
	h.notifyPipeline(req.Request.Context(), "silence:delete")
	httputil.Success(resp, nil)
}

// ─── Notification Policies ─────────────────────────────────────────────────────

// policyWithCount embeds NotificationPolicy with a computed linked-rule count.
type policyWithCount struct {
	model.NotificationPolicy
	LinkedRules int64 `json:"linked_rules"`
}

func (h *alertCenterHandler) listPolicies(req *restful.Request, resp *restful.Response) {
	var rows []model.NotificationPolicy
	h.db.WithContext(req.Request.Context()).Order("created_at desc").Find(&rows)

	result := make([]policyWithCount, 0, len(rows))
	for _, r := range rows {
		var count int64
		// Count alert_routes whose channel_ids JSON array contains this policy id
		h.db.WithContext(req.Request.Context()).
			Model(&model.AlertRoute{}).
			Where("channel_ids @> ?", `["`+r.ID+`"]`).
			Count(&count)
		result = append(result, policyWithCount{NotificationPolicy: r, LinkedRules: count})
	}
	httputil.Success(resp, result)
}

// normalizePolicyJSON makes sure the three jsonb columns satisfy their
// NOT NULL DEFAULT '[]' constraint when the client omits them.
func normalizePolicyJSON(p *model.NotificationPolicy) {
	p.Severities = ensureJSONArray(p.Severities)
	p.ContactIDs = ensureJSONArray(p.ContactIDs)
	p.GroupIDs = ensureJSONArray(p.GroupIDs)
}

func (h *alertCenterHandler) createPolicy(req *restful.Request, resp *restful.Response) {
	var row model.NotificationPolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	normalizePolicyJSON(&row)
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updatePolicy(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.NotificationPolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	normalizePolicyJSON(&row)
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deletePolicy(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.NotificationPolicy{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, nil)
}

// ─── Notification Contacts ─────────────────────────────────────────────────────

func (h *alertCenterHandler) listContacts(req *restful.Request, resp *restful.Response) {
	var rows []model.NotificationContact
	h.db.WithContext(req.Request.Context()).Find(&rows)
	// Mask secrets in list responses
	for i := range rows {
		rows[i].WebhookToken = maskSecret(rows[i].WebhookToken)
		rows[i].SlackBotToken = maskSecret(rows[i].SlackBotToken)
		rows[i].FeishuSecret = maskSecret(rows[i].FeishuSecret)
		rows[i].DingtalkSecret = maskSecret(rows[i].DingtalkSecret)
	}
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createContact(req *restful.Request, resp *restful.Response) {
	var row model.NotificationContact
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	decodeContactClientCiphers(&row)
	h.encryptContactSecrets(&row)
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	maskContactSecretsInPlace(&row)
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateContact(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.NotificationContact
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	// RSA-decrypt any client-side encrypted values first.
	decodeContactClientCiphers(&row)
	// If a secret field is "******" (masked), keep the existing DB value;
	// otherwise re-encrypt at rest with AES.
	var existing model.NotificationContact
	if err := h.db.WithContext(req.Request.Context()).Where("id = ?", id).First(&existing).Error; err == nil {
		if row.WebhookToken == "******" {
			row.WebhookToken = existing.WebhookToken
		} else {
			row.WebhookToken = h.encryptField(row.WebhookToken)
		}
		if row.SlackBotToken == "******" {
			row.SlackBotToken = existing.SlackBotToken
		} else {
			row.SlackBotToken = h.encryptField(row.SlackBotToken)
		}
		if row.FeishuSecret == "******" {
			row.FeishuSecret = existing.FeishuSecret
		} else {
			row.FeishuSecret = h.encryptField(row.FeishuSecret)
		}
		if row.DingtalkSecret == "******" {
			row.DingtalkSecret = existing.DingtalkSecret
		} else {
			row.DingtalkSecret = h.encryptField(row.DingtalkSecret)
		}
	} else {
		h.encryptContactSecrets(&row)
	}
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	maskContactSecretsInPlace(&row)
	httputil.Success(resp, row)
}

// maskContactSecretsInPlace replaces non-empty secret fields with the
// placeholder used by the UI ("******").  Returned to the browser so it
// never sees raw secret material in the response body.
func maskContactSecretsInPlace(c *model.NotificationContact) {
	c.WebhookToken = maskSecret(c.WebhookToken)
	c.SlackBotToken = maskSecret(c.SlackBotToken)
	c.FeishuSecret = maskSecret(c.FeishuSecret)
	c.DingtalkSecret = maskSecret(c.DingtalkSecret)
}

func (h *alertCenterHandler) deleteContact(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.NotificationContact{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, nil)
}

// ─── Notification Contact Groups ───────────────────────────────────────────────

func (h *alertCenterHandler) listContactGroups(req *restful.Request, resp *restful.Response) {
	var rows []model.NotificationContactGroup
	h.db.WithContext(req.Request.Context()).Find(&rows)
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createContactGroup(req *restful.Request, resp *restful.Response) {
	var row model.NotificationContactGroup
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	row.ContactIDs = ensureJSONArray(row.ContactIDs)
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateContactGroup(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.NotificationContactGroup
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	row.ContactIDs = ensureJSONArray(row.ContactIDs)
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteContactGroup(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.NotificationContactGroup{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, nil)
}

// ─── Inhibit Rules ─────────────────────────────────────────────────────────────

func (h *alertCenterHandler) listInhibits(req *restful.Request, resp *restful.Response) {
	var rows []model.InhibitRule
	h.db.WithContext(req.Request.Context()).Order("created_at desc").Find(&rows)
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createInhibit(req *restful.Request, resp *restful.Response) {
	var row model.InhibitRule
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	row.Equal = ensureJSONArray(row.Equal)
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "inhibit:create")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateInhibit(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.InhibitRule
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	row.Equal = ensureJSONArray(row.Equal)
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "inhibit:update")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteInhibit(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.InhibitRule{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "inhibit:delete")
	httputil.Success(resp, nil)
}

// ─── Escalation Policies ───────────────────────────────────────────────────────

func (h *alertCenterHandler) listEscalations(req *restful.Request, resp *restful.Response) {
	var rows []model.EscalationPolicy
	h.db.WithContext(req.Request.Context()).Order("created_at desc").Find(&rows)
	httputil.Success(resp, rows)
}

func (h *alertCenterHandler) createEscalation(req *restful.Request, resp *restful.Response) {
	var row model.EscalationPolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = ""
	if err := h.db.WithContext(req.Request.Context()).Create(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "escalation:create")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) updateEscalation(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	var row model.EscalationPolicy
	if err := req.ReadEntity(&row); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	row.ID = id
	if err := h.db.WithContext(req.Request.Context()).Save(&row).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "escalation:update")
	httputil.Success(resp, row)
}

func (h *alertCenterHandler) deleteEscalation(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	if err := h.db.WithContext(req.Request.Context()).Delete(&model.EscalationPolicy{}, "id = ?", id).Error; err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	h.notifyPipeline(req.Request.Context(), "escalation:delete")
	httputil.Success(resp, nil)
}
