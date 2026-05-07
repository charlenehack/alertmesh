package router

import (
	restful "github.com/emicklei/go-restful/v3"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/auth"
	"github.com/kuzane/alertmesh/internal/httputil"
	"github.com/kuzane/alertmesh/internal/label"
	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/internal/sysconfig"
)

type systemHandler struct {
	db     *gorm.DB
	jwtSvc *auth.JWTService
	syscfg *sysconfig.Service
}

func newSystemHandler(db *gorm.DB, jwtSvc *auth.JWTService, syscfg *sysconfig.Service) *systemHandler {
	return &systemHandler{db: db, jwtSvc: jwtSvc, syscfg: syscfg}
}

func (h *systemHandler) registerRoutes(ws *restful.WebService) {
	// Public – no auth, no ACL
	ws.Route(ws.GET("/auth/public-key").
		To(h.getPublicKey).
		Doc("Get RSA public key for password encryption"))

	ws.Route(ws.POST("/auth/login").
		To(h.login).
		Doc("User login"))

	// User info (auth required, no ACL)
	ws.Route(ws.GET("/user/info").
		To(h.userInfo).
		Doc("Get current user info and permissions").
		Metadata(label.MetaAuth, label.Enable))

	// User management
	ws.Route(ws.GET("/users").
		To(h.listUsers).
		Doc("List users").
		Metadata(label.MetaIdentity, label.UserRead).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "User").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// System config (generic k/v, non-secret)
	ws.Route(ws.GET("/configs").
		To(h.listConfigs).
		Doc("List non-secret system configs").
		Metadata(label.MetaIdentity, label.ConfigRead).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "Config").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/configs").
		To(h.updateConfig).
		Doc("Update a system config value").
		Metadata(label.MetaIdentity, label.ConfigWrite).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "Config").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// Auth provider configuration
	ws.Route(ws.GET("/configs/auth").
		To(h.getAuthConfig).
		Doc("Get current authentication mode").
		Metadata(label.MetaIdentity, label.ConfigRead).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "Config").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	ws.Route(ws.PUT("/configs/auth").
		To(h.setAuthConfig).
		Doc("Set authentication mode and provider config (ldap/oidc config is encrypted at rest)").
		Metadata(label.MetaIdentity, label.ConfigWrite).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "Config").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// Oncall
	ws.Route(ws.GET("/oncall").
		To(h.listOncall).
		Doc("List oncall schedules").
		Metadata(label.MetaIdentity, label.OncallRead).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "Oncall").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))

	// Reports
	ws.Route(ws.GET("/reports/overview").
		To(h.reportOverview).
		Doc("Get overview report").
		Metadata(label.MetaIdentity, label.ReportRead).
		Metadata(label.MetaModule, label.SysModuleName).
		Metadata(label.MetaKind, "Report").
		Metadata(label.MetaAuth, label.Enable).
		Metadata(label.MetaACL, label.Enable))
}

// ─── auth ────────────────────────────────────────────────────────────────────

func (h *systemHandler) getPublicKey(_ *restful.Request, resp *restful.Response) {
	httputil.Success(resp, map[string]string{"public_key": auth.GetPublicKeyPEM()})
}

func (h *systemHandler) login(req *restful.Request, resp *restful.Response) {
	type loginReq struct {
		Username string `json:"username"`
		// Password is RSA-PKCS1v15 encrypted and base64-standard-encoded by the
		// browser using the public key from GET /auth/public-key.
		Password string `json:"password"`
	}
	var body loginReq
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}

	// Decrypt the RSA-encrypted password sent by the browser.
	plainPassword, err := auth.DecryptCipher(body.Password)
	if err != nil {
		httputil.BadRequest(resp, "invalid password encoding")
		return
	}

	localAuth := auth.NewLocalAuth(h.db)
	user, err := localAuth.Authenticate(req.Request.Context(), body.Username, plainPassword)
	if err != nil {
		httputil.Unauthorized(resp)
		return
	}

	roleNames := make([]string, 0, len(user.Roles))
	for _, r := range user.Roles {
		roleNames = append(roleNames, r.Name)
	}

	token, err := h.jwtSvc.GenerateToken(user.ID, user.Username, roleNames)
	if err != nil {
		httputil.InternalError(resp, "failed to generate token")
		return
	}

	httputil.Success(resp, map[string]string{"token": token})
}

// ─── user ────────────────────────────────────────────────────────────────────

// safeUser is the API-facing user representation with sensitive fields masked.
type safeUser struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"` // masked
	DisplayName string     `json:"display_name"`
	Source      string     `json:"source"`
	IsActive    bool       `json:"is_active"`
	Roles       []safeRole `json:"roles,omitempty"`
}

type safeRole struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func toSafeUser(u model.User) safeUser {
	roles := make([]safeRole, 0, len(u.Roles))
	for _, r := range u.Roles {
		roles = append(roles, safeRole{ID: r.ID, Name: r.Name})
	}
	return safeUser{
		ID:          u.ID,
		Username:    u.Username,
		Email:       httputil.MaskEmail(u.Email),
		DisplayName: u.DisplayName,
		Source:      u.Source,
		IsActive:    u.IsActive,
		Roles:       roles,
	}
}

func (h *systemHandler) userInfo(req *restful.Request, resp *restful.Response) {
	username, _ := req.Attribute("username").(string)
	roles, _ := req.Attribute("roles").([]string)
	permissions := h.resolvePermissions(roles)

	httputil.Success(resp, map[string]interface{}{
		"username":    username,
		"roles":       roles,
		"permissions": permissions,
	})
}

func (h *systemHandler) resolvePermissions(roles []string) []string {
	for _, r := range roles {
		if r == "admin" || r == "superadmin" {
			return []string{"*"}
		}
	}

	var endpoints []model.Endpoint
	h.db.Joins("JOIN role_endpoints ON role_endpoints.endpoint_identity = endpoints.identity").
		Joins("JOIN roles ON roles.id = role_endpoints.role_id").
		Where("roles.name IN ?", roles).
		Find(&endpoints)

	seen := make(map[string]bool)
	var perms []string
	for _, ep := range endpoints {
		if !seen[ep.Identity] {
			seen[ep.Identity] = true
			perms = append(perms, ep.Identity)
		}
	}
	return perms
}

func (h *systemHandler) listUsers(req *restful.Request, resp *restful.Response) {
	var users []model.User
	h.db.WithContext(req.Request.Context()).Preload("Roles").Find(&users)

	safe := make([]safeUser, 0, len(users))
	for _, u := range users {
		safe = append(safe, toSafeUser(u))
	}
	httputil.Success(resp, safe)
}

// ─── generic config ──────────────────────────────────────────────────────────

// listConfigs returns only the non-secret keys (excludes security.* and auth.ldap/oidc).
func (h *systemHandler) listConfigs(req *restful.Request, resp *restful.Response) {
	var configs []model.SystemConfig
	h.db.WithContext(req.Request.Context()).
		Where("key NOT LIKE 'security.%' AND key NOT IN ('auth.ldap','auth.oidc')").
		Find(&configs)
	httputil.Success(resp, configs)
}

func (h *systemHandler) updateConfig(req *restful.Request, resp *restful.Response) {
	var cfg model.SystemConfig
	if err := req.ReadEntity(&cfg); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	// Prevent direct writes to secret keys via this generic endpoint.
	if cfg.Key == sysconfig.KeyJWTSecret || cfg.Key == sysconfig.KeyAuthLDAP || cfg.Key == sysconfig.KeyAuthOIDC {
		httputil.BadRequest(resp, "use /configs/auth to manage sensitive configuration")
		return
	}
	h.db.WithContext(req.Request.Context()).Save(&cfg)
	httputil.Success(resp, cfg)
}

// ─── auth config ─────────────────────────────────────────────────────────────

func (h *systemHandler) getAuthConfig(req *restful.Request, resp *restful.Response) {
	mode := h.syscfg.AuthMode(req.Request.Context())
	httputil.Success(resp, map[string]string{"mode": mode})
}

type setAuthConfigReq struct {
	// Mode is one of: local | ldap | oidc
	Mode string `json:"mode"`
	// Config holds the provider-specific JSON payload.
	// For "ldap": { host, port, base_dn, bind_dn, bind_password, user_filter, tls }
	// For "oidc": { issuer, client_id, client_secret, redirect_uri, scopes[] }
	// Omit for "local".  The value is encrypted before storage.
	Config string `json:"config,omitempty"`
}

func (h *systemHandler) setAuthConfig(req *restful.Request, resp *restful.Response) {
	var body setAuthConfigReq
	if err := req.ReadEntity(&body); err != nil {
		httputil.BadRequest(resp, "invalid request body")
		return
	}
	switch body.Mode {
	case "local", "ldap", "oidc":
	default:
		httputil.BadRequest(resp, "mode must be one of: local, ldap, oidc")
		return
	}
	// Walk the JSON config and RSA-decrypt any field the browser wrapped with
	// auth.ENCPrefix (e.g. ldap.bind_password / oidc.client_secret) before it
	// is AES-encrypted at rest by sysconfig.SetAuthMode.
	configJSON := auth.DecodeJSONClientCiphers(body.Config)
	if err := h.syscfg.SetAuthMode(req.Request.Context(), body.Mode, configJSON); err != nil {
		httputil.InternalError(resp, "failed to update auth config")
		return
	}
	httputil.Success(resp, map[string]string{"mode": body.Mode})
}

// ─── oncall / reports ────────────────────────────────────────────────────────

func (h *systemHandler) listOncall(req *restful.Request, resp *restful.Response) {
	var schedules []model.OncallSchedule
	h.db.WithContext(req.Request.Context()).Find(&schedules)
	httputil.Success(resp, schedules)
}

func (h *systemHandler) reportOverview(req *restful.Request, resp *restful.Response) {
	httputil.Success(resp, map[string]string{"status": "not yet implemented"})
}
