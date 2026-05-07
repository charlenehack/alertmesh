package middleware

import (
	"net/http"
	"strings"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"

	"github.com/kuzane/alertmesh/internal/auth"
)

// AuthFilter extracts and validates the JWT token for routes that require authentication.
func AuthFilter(jwtSvc *auth.JWTService) restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		meta := req.SelectedRoute().Metadata()
		if !isEnabled(meta, "auth") {
			chain.ProcessFilter(req, resp)
			return
		}

		header := req.HeaderParameter("Authorization")
		// Browsers can't set custom headers on `new WebSocket(url)`, so
		// the canonical escape hatch for those routes is a `?token=`
		// query parameter.  We fall back to it only when the
		// Authorization header is absent — anyone who can set the
		// header (REST clients, server-to-server callers, curl) keeps
		// working unchanged.  The token is logged at audit-debug
		// granularity at most: NewAuditFilter records URL.Path only,
		// not RawQuery, so `?token=` does not leak into audit_logs.
		if header == "" {
			if t := req.QueryParameter("token"); t != "" {
				header = "Bearer " + t
			}
		}
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			_ = resp.WriteErrorString(http.StatusUnauthorized, "missing or invalid authorization header")
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := jwtSvc.ParseToken(tokenStr)
		if err != nil {
			log.Debug().Err(err).Msg("jwt parse failed")
			_ = resp.WriteErrorString(http.StatusUnauthorized, "invalid token")
			return
		}

		req.SetAttribute("username", claims.Username)
		req.SetAttribute("user_id", claims.UserID)
		req.SetAttribute("roles", claims.Roles)

		chain.ProcessFilter(req, resp)
	}
}

func isEnabled(meta map[string]interface{}, key string) bool {
	v, ok := meta[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
