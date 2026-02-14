package app

import (
	"context"
	"fmt"
	"path/filepath"

	"insightify/internal/gateway/config"
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/handler/rpc"
	"insightify/internal/gateway/repository/projectstore"
	"insightify/internal/gateway/repository/ui"
	"insightify/internal/gateway/server"
	gatewayproject "insightify/internal/gateway/service/project"
	gatewayrun "insightify/internal/gateway/service/run"
)

type App struct {
	server *server.Server
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Dependencies
	defaultProjectStore := projectstore.NewFromEnv(filepath.Join("tmp", "project_states.json"))
	uiStore := ui.NewStore()
	projectSvc := gatewayproject.New(defaultProjectStore)
	runSvc := gatewayrun.New(projectSvc.AsProjectReader(), uiStore)

	projectHandler := rpc.NewProjectHandler(projectSvc)
	runHandler := rpc.NewRunHandler(runSvc)
	userInteractionHandler := rpc.NewUserInteractionHandler(runSvc)
	traceHandler := handler.NewTraceHandler(runSvc)

	// Routing & Server
	mux := server.NewMux(projectHandler, runHandler, userInteractionHandler, traceHandler)
	srv := server.New(cfg.Port, mux)

	return &App{
		server: srv,
	}, nil
}

func (a *App) Start() error {
	return a.server.Start()
}

func (a *App) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}
