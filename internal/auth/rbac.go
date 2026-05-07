package auth

import (
	"github.com/mikespook/gorbac"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
			// gorbac StdRole.Assign only fails on a nil permission, which we
			// never construct — the Permissioner always wraps a non-empty
			// "METHOD:/path" string.  Discarding the error keeps the loop tight.
			_ = stdRole.Assign(gorbac.NewStdPermission(ep.Method + ":" + ep.Path))
		}
		if err := rbac.Add(stdRole); err != nil {
			// Role may already exist from a previous load, remove and re-add.
			// Both calls only fail with "role missing"/"role already exists",
			// which we are explicitly trying to overwrite — log and continue.
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
func StoreRouter(container *restful.Container, db *gorm.DB) {
	var endpoints []model.Endpoint
	for _, ws := range container.RegisteredWebServices() {
		for _, route := range ws.Routes() {
			if !isACLEnabled(route.Metadata) {
				continue
			}
			endpoints = append(endpoints, model.Endpoint{
				Path:     route.Path,
				Method:   route.Method,
				Module:   metaString(route.Metadata, "module"),
				Kind:     metaString(route.Metadata, "kind"),
				Identity: metaString(route.Metadata, "identity"),
				Remark:   route.Doc,
			})
		}
	}
	if len(endpoints) == 0 {
		return
	}
	db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&endpoints)
	log.Info().Int("endpoints", len(endpoints)).Msg("router endpoints synced")
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
