package auth

import (
	"github.com/mikespook/gorbac"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	restful "github.com/emicklei/go-restful/v3"

	"github.com/kuzane/alertmesh/internal/model"
)

// InitRBAC loads roles and endpoints from the database and builds the gorbac in-memory graph.
func InitRBAC(rbac *gorbac.RBAC, db *gorm.DB) error {
	return SetRBAC(rbac, db)
}

// SetRBAC rebuilds the gorbac in-memory permission graph from the database.
func SetRBAC(rbac *gorbac.RBAC, db *gorm.DB) error {
	var roles []model.Role
	if err := db.Preload("Endpoints").Where("status = ?", true).Find(&roles).Error; err != nil {
		return err
	}

	for _, role := range roles {
		stdRole := gorbac.NewStdRole(role.Name)
		for _, ep := range role.Endpoints {
			_ = stdRole.Assign(gorbac.NewStdPermission(ep.Method + ":" + ep.Path))
		}
		if err := rbac.Add(stdRole); err != nil {
			if rmErr := rbac.Remove(role.Name); rmErr != nil {
				log.Warn().Err(rmErr).Str("role", role.Name).Msg("rbac remove during reload")
			}
			if addErr := rbac.Add(stdRole); addErr != nil {
				log.Warn().Err(addErr).Str("role", role.Name).Msg("rbac re-add during reload")
			}
		}
	}

	for _, role := range roles {
		if len(role.Parents) > 0 {
			if err := rbac.SetParents(role.Name, role.Parents); err != nil {
				log.Warn().Err(err).Str("role", role.Name).Msg("failed to set role parents")
			}
		}
	}

	log.Info().Int("roles", len(roles)).Msg("rbac graph built")
	return nil
}

// StoreRouter synchronises go-restful routes marked with acl=true into the endpoints table.
// Only removes stale endpoints (routes that no longer exist) and preserves existing role-endpoint
// associations for endpoints that still exist. New endpoints are added without touching existing
// role permissions.
func StoreRouter(container *restful.Container, db *gorm.DB) {
	var endpoints []model.Endpoint
	seen := make(map[string]bool)
	for _, ws := range container.RegisteredWebServices() {
		for _, route := range ws.Routes() {
			if !isACLEnabled(route.Metadata) {
				continue
			}
			identity := metaString(route.Metadata, "identity")
			if identity == "" || seen[identity] {
				continue
			}
			seen[identity] = true
			endpoints = append(endpoints, model.Endpoint{
				Path:     route.Path,
				Method:   route.Method,
				Module:   metaString(route.Metadata, "module"),
				Kind:     metaString(route.Metadata, "kind"),
				Identity: identity,
				Remark:   route.Doc,
			})
		}
	}
	if len(endpoints) == 0 {
		return
	}

	// Build identity set of current routes
	newIdents := make(map[string]bool, len(endpoints))
	for _, ep := range endpoints {
		newIdents[ep.Identity] = true
	}

	// Find stale endpoints (exist in DB but no longer in routes) and remove them
	var staleIdents []string
	if err := db.Model(&model.Endpoint{}).Pluck("identity", &staleIdents).Error; err != nil {
		log.Warn().Err(err).Msg("pluck existing identities failed")
	}
	for _, ident := range staleIdents {
		if !newIdents[ident] {
			// Remove role_endpoints referencing stale endpoint first (FK constraint)
			if err := db.Where("endpoint_identity = ?", ident).Delete(&model.RoleEndpoint{}).Error; err != nil {
				log.Warn().Err(err).Str("identity", ident).Msg("remove stale role_endpoint failed")
			}
			if err := db.Where("identity = ?", ident).Delete(&model.Endpoint{}).Error; err != nil {
				log.Warn().Err(err).Str("identity", ident).Msg("remove stale endpoint failed")
			}
		}
	}

	// Upsert: only create new endpoints, update existing ones' path/method/module/kind/remark
	for _, ep := range endpoints {
		result := db.Where("identity = ?", ep.Identity).Assign(map[string]any{
			"path": ep.Path, "method": ep.Method, "module": ep.Module, "kind": ep.Kind, "remark": ep.Remark,
		}).FirstOrCreate(&ep)
		if result.Error != nil {
			log.Warn().Err(result.Error).Str("identity", ep.Identity).Msg("upsert endpoint failed")
		}
	}

	log.Info().Int("endpoints", len(endpoints)).Msg("router endpoints synced (preserved role permissions)")
}

func isACLEnabled(meta map[string]interface{}) bool {
	v, ok := meta["acl"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func metaString(meta map[string]interface{}, key string) string {
	v, ok := meta[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}