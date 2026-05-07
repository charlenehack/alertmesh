package main

import (
	"net/http"

	restful "github.com/emicklei/go-restful/v3"
	"gorm.io/gorm"

	"github.com/kuzane/alertmesh/internal/ai"
	"github.com/kuzane/alertmesh/internal/auth"
	"github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/engine"
	"github.com/kuzane/alertmesh/internal/incident"
	"github.com/kuzane/alertmesh/internal/realtime"
)

// App holds the top-level dependencies needed to run and shut down the service.
type App struct {
	Cfg             *config.Config
	Server          *http.Server
	Orchestrator    *ai.Orchestrator
	Pipeline        *engine.Pipeline
	IncidentService *incident.Service
	DB              *gorm.DB
	// RealtimeHub is the topic-based pub/sub used by the WS push channel
	// that replaces every UI polling timer.  Held here so main.go can
	// kick off realtime.Start(rootCtx, db, hub) alongside the other
	// long-lived background loops (orchestrator, pipeline reload listener).
	RealtimeHub *realtime.Hub
}

// NewApp is the final Wire provider.  It registers all HTTP routes into the
// endpoint table after the container is fully built, then returns the runnable
// App value.
func NewApp(
	cfg *config.Config,
	server *http.Server,
	orchestrator *ai.Orchestrator,
	pipeline *engine.Pipeline,
	incSvc *incident.Service,
	db *gorm.DB,
	rtHub *realtime.Hub,
	container *restful.Container,
) *App {
	auth.StoreRouter(container, db)
	return &App{
		Cfg:             cfg,
		Server:          server,
		Orchestrator:    orchestrator,
		Pipeline:        pipeline,
		IncidentService: incSvc,
		DB:              db,
		RealtimeHub:     rtHub,
	}
}
