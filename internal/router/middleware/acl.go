package middleware

import (
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/mikespook/gorbac"
	"github.com/rs/zerolog/log"
)

// ACLFilter checks whether the authenticated user has permission to access the route.
func ACLFilter(rbac *gorbac.RBAC) restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		if !isEnabled(req.SelectedRoute().Metadata(), "acl") {
			chain.ProcessFilter(req, resp)
			return
		}

		username, _ := req.Attribute("username").(string)
		if username == "admin" {
			chain.ProcessFilter(req, resp)
			return
		}

		roles, _ := req.Attribute("roles").([]string)
		permFlag := req.Request.Method + ":" + req.SelectedRoute().Path()

		granted := false
		for _, roleName := range roles {
			if rbac.IsGranted(roleName, gorbac.NewStdPermission(permFlag), nil) {
				granted = true
				break
			}
		}

		if !granted {
			log.Warn().Str("user", username).Str("perm", permFlag).Msg("permission denied")
			_ = resp.WriteErrorString(http.StatusForbidden, "permission denied")
			return
		}

		chain.ProcessFilter(req, resp)
	}
}
