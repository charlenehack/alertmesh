package middleware

import (
	"encoding/json"
	"net"
	"strings"
	"time"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/model"
	"github.com/kuzane/alertmesh/pkg/metrics"
)

// NewAuditFilter returns a filter that logs API requests to both zerolog and the
// audit_logs database table. Write operations (POST/PUT/DELETE) are persisted;
// GET requests are only logged to zerolog to avoid flooding the table.
// It also tracks Prometheus request metrics (count + duration).
func NewAuditFilter(db *gorm.DB) restful.FilterFunction {
	return func(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
		start := time.Now()

		chain.ProcessFilter(req, resp)

		method := req.Request.Method
		path := req.Request.URL.Path
		username, _ := req.Attribute("username").(string)
		userID, _ := req.Attribute("user_id").(string)
		statusCode := resp.StatusCode()
		duration := time.Since(start)

		// Prometheus metrics
		routePath := path
		if sel := req.SelectedRoute(); sel != nil {
			routePath = sel.Path()
		}
		metrics.HTTPRequests.WithLabelValues(method, routePath, statusText(statusCode)).Inc()
		metrics.HTTPDuration.WithLabelValues(method, routePath).Observe(duration.Seconds())

		log.Info().
			Str("component", "audit").
			Str("method", method).
			Str("path", path).
			Str("user", username).
			Int("status", statusCode).
			Dur("duration", duration).
			Msg("api request")

		// Only persist write operations to DB
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			return
		}

		detail, _ := json.Marshal(map[string]any{
			"method":      method,
			"status":      statusCode,
			"duration_ms": duration.Milliseconds(),
		})

		go db.Create(&model.AuditLog{
			UserID:   userID,
			Username: username,
			Action:   method + " " + path,
			Resource: extractResource(path),
			Detail:   detail,
			IP:       clientIP(req),
		})
	}
}

func clientIP(req *restful.Request) string {
	if xff := req.HeaderParameter("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := req.HeaderParameter("X-Real-Ip"); xri != "" {
		return xri
	}
	host, _, _ := net.SplitHostPort(req.Request.RemoteAddr)
	return host
}

func extractResource(path string) string {
	// Strip whichever API version prefix is present so /api/v1/alerts/* and
	// /api/v2/alerts both bucket under "alerts" in audit_logs.resource —
	// otherwise the v2 Prometheus push endpoint shows up as "v2" which is
	// useless for filtering.
	for _, prefix := range []string{"/api/v1/", "/api/v2/"} {
		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return path
}

func statusText(code int) string {
	switch {
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}
