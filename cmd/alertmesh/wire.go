//go:build wireinject

package main

import (
	"context"

	"github.com/google/wire"
	"github.com/kuzane/alertmesh/internal/ai"
	"github.com/kuzane/alertmesh/internal/config"
	"github.com/kuzane/alertmesh/internal/engine"
	"github.com/kuzane/alertmesh/internal/realtime"
	"github.com/kuzane/alertmesh/internal/router"
)

// InitApp is the Wire entry-point.  cfg is loaded and the logger is
// initialised in main before this is called.  rootCtx is the long-lived
// application context (cancelled by main on SIGTERM); it is threaded
// into incident.Service and ai.Orchestrator so their fire-and-forget
// goroutines respect graceful shutdown.
func InitApp(rootCtx context.Context, cfg *config.Config) (*App, error) {
	wire.Build(
		ProvideDB,
		ProvideSysConfig,
		ProvideJWTService,
		ProvideRBAC,
		ProvideDispatcher,
		ProvideIncidentService,
		ProvideIncidentCallback,
		ProvideResolvedCallback,
		ProvideAIDispatchHook,
		engine.NewPipeline,
		ai.NewWSHub,
		ai.NewOrchestrator,
		realtime.NewHub,
		router.Setup,
		ProvideHTTPServer,
		NewApp,
	)
	return nil, nil
}
