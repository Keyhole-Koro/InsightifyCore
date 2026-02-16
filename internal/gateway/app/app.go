package app

import (
	"context"
	"fmt"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"insightify/internal/gateway/config"
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/handler/rpc"
	"insightify/internal/gateway/handler/ws"
	"insightify/internal/gateway/repository/projectstore"
	"insightify/internal/gateway/server"
	gatewayproject "insightify/internal/gateway/service/project"
	gatewayui "insightify/internal/gateway/service/ui"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
	gatewayuserinteraction "insightify/internal/gateway/service/userinteraction"
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

	defaultProjectStore := projectstore.NewFromEnv(filepath.Join("tmp", "project_states.json"))
	stores, err := initStores(cfg)
	if err != nil {
		return nil, err
	}

	projectSvc := gatewayproject.New(defaultProjectStore)
	uiWorkspaceSvc := gatewayuiworkspace.New(stores.uiWorkspace)
	uiSvc := gatewayui.New(stores.ui, uiWorkspaceSvc, stores.artifact, cfg.Interaction.ConversationArtifactPath)
	userInteractionSvc := gatewayuserinteraction.New(stores.artifact, cfg.Interaction.ConversationArtifactPath)
	workerSvc := gatewayworker.New(projectSvc.AsProjectReader(), uiWorkspaceSvc, uiSvc, userInteractionSvc, stores.artifact)

	projectHandler := rpc.NewProjectHandler(projectSvc)
	runHandler := rpc.NewRunHandler(workerSvc)
	userInteractionHandler := ws.NewUserInteractionHandler(userInteractionSvc)
	uiHandler := rpc.NewUiHandler(uiSvc)
	uiWorkspaceHandler := rpc.NewUiWorkspaceHandler(uiSvc)
	traceHandler := handler.NewTraceHandler(workerSvc)

	// Routing & Server
	mux := server.NewMux(projectHandler, runHandler, userInteractionHandler, uiHandler, uiWorkspaceHandler, traceHandler)
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
