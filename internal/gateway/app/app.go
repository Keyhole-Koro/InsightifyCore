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
	gatewayui "insightify/internal/gateway/service/ui"
	gatewayuserinteraction "insightify/internal/gateway/service/user_interaction"
	gatewayworker "insightify/internal/gateway/service/worker"
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
	uiSvc := gatewayui.New(uiStore)
	projectSvc := gatewayproject.New(defaultProjectStore)
	workerSvc := gatewayworker.New(projectSvc.AsProjectReader(), uiSvc)
	userInteractionSvc := gatewayuserinteraction.New()

	projectHandler := rpc.NewProjectHandler(projectSvc)
	runHandler := rpc.NewRunHandler(workerSvc)
	userInteractionHandler := rpc.NewUserInteractionHandler(userInteractionSvc)
	traceHandler := handler.NewTraceHandler(workerSvc)

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
