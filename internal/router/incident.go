package router

import (
	"net/http"
	"strconv"

	restful "github.com/emicklei/go-restful/v3"

	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/incident"
	"github.com/kuzane/alertmesh/internal/label"
)

type incidentHandler struct {
	svc *incident.Service
}

func newIncidentHandler(svc *incident.Service) *incidentHandler {
	return &incidentHandler{svc: svc}
}

func (h *incidentHandler) registerRoutes(ws *restful.WebService) {
	ws.Route(ws.GET("/incidents").
		To(h.list).
		Doc("List incidents").
		Metadata(label.MetaIdentity, label.IncidentList).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "Incident").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.GET("/incidents/{id}").
		To(h.detail).
		Doc("Get incident detail").
		Metadata(label.MetaIdentity, label.IncidentDetail).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "Incident").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/incidents/{id}/ack").
		To(h.ack).
		Doc("Acknowledge incident").
		Metadata(label.MetaIdentity, label.IncidentAck).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "Incident").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/incidents/{id}/resolve").
		To(h.resolve).
		Doc("Resolve incident").
		Metadata(label.MetaIdentity, label.IncidentResolve).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "Incident").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.POST("/incidents/{id}/close").
		To(h.close).
		Doc("Close incident").
		Metadata(label.MetaIdentity, label.IncidentClose).
		Metadata(label.MetaModule, label.IncidentModuleName).
		Metadata(label.MetaKind, "Incident").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))
}

func (h *incidentHandler) list(req *restful.Request, resp *restful.Response) {
	offset, _ := strconv.Atoi(req.QueryParameter("offset"))
	limit, _ := strconv.Atoi(req.QueryParameter("limit"))
	if limit <= 0 {
		limit = 20
	}

	items, total, err := h.svc.List(req.Request.Context(), offset, limit)
	if err != nil {
		httputil.InternalError(resp, err.Error())
		return
	}
	httputil.Success(resp, httputil.PagedData{Items: items, Total: total})
}

func (h *incidentHandler) detail(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	inc, err := h.svc.GetByID(req.Request.Context(), id)
	if err != nil {
		httputil.NotFound(resp)
		return
	}
	httputil.Success(resp, inc)
}

func (h *incidentHandler) ack(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	userID, _ := req.Attribute("user_id").(string)
	username, _ := req.Attribute("username").(string)
	if err := h.svc.Ack(req.Request.Context(), id, userID, username); err != nil {
		httputil.Error(resp, http.StatusBadRequest, err.Error())
		return
	}
	httputil.Success(resp, nil)
}

func (h *incidentHandler) resolve(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	userID, _ := req.Attribute("user_id").(string)
	username, _ := req.Attribute("username").(string)
	if err := h.svc.Resolve(req.Request.Context(), id, userID, username); err != nil {
		httputil.Error(resp, http.StatusBadRequest, err.Error())
		return
	}
	httputil.Success(resp, nil)
}

func (h *incidentHandler) close(req *restful.Request, resp *restful.Response) {
	id := req.PathParameter("id")
	userID, _ := req.Attribute("user_id").(string)
	username, _ := req.Attribute("username").(string)
	if err := h.svc.Close(req.Request.Context(), id, userID, username); err != nil {
		httputil.Error(resp, http.StatusBadRequest, err.Error())
		return
	}
	httputil.Success(resp, nil)
}
