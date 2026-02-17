package app

import (
	"context"
	"fmt"
	"path/filepath"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"insightify/internal/gateway/config"
	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/handler/rpc"
	"insightify/internal/gateway/handler/ws"
	"insightify/internal/gateway/repository/artifact"
	projectrepo "insightify/internal/gateway/repository/project"
	"insightify/internal/gateway/repository/ui"
	"insightify/internal/gateway/repository/uiworkspace"
	"insightify/internal/gateway/server"
	gatewayproject "insightify/internal/gateway/service/project"
	gatewayui "insightify/internal/gateway/service/ui"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
	gatewayuserinteraction "insightify/internal/gateway/service/userinteraction"
	gatewayworker "insightify/internal/gateway/service/worker"
)

type App struct {
	server    *server.Server
	entClient *ent.Client // Add Ent client to App struct for proper shutdown
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize Postgres connection pool
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)

	// Initialize Ent client
	drv := entsql.OpenDB("pgx", db)
	client := ent.NewClient(ent.Driver(drv))

	// Run Ent migrations
	if err := client.Schema.Create(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}

	// Repositories
	// Artifacts (mixed)
	artifactConfig := artifact.S3Config{
		Endpoint:  cfg.Artifact.Endpoint,
		AccessKey: cfg.Artifact.AccessKey,
		SecretKey: cfg.Artifact.SecretKey,
		Bucket:    cfg.Artifact.Bucket,
		Region:    cfg.Artifact.Region,
		UseSSL:    cfg.Artifact.UseSSL,
	}
	artifactStore, err := artifact.NewS3Store(artifactConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact store: %w", err)
	}

	// Project Store (Ent) with Cache (nil for now or initialize if needed)
	// Passing nil for cache as we haven't initialized it here, or we can use generic LRU if import available.
	// For simplicity and to fix build, we pass nil. Store handles nil cache gracefully.
	projectStore, err := projectrepo.NewPostgresStore(client, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create project store: %w", err)
	}

	// UI Store (Ent)
	uiStore := ui.NewPostgresStore(client)

	// Workspace Store (Ent)
	uiWorkspaceStore := uiworkspace.NewPostgresStore(client)

	_ = projectrepo.NewFromEnv(filepath.Join("tmp", "project_states.json")) // kept for local fallback wiring evolution

	projectSvc := gatewayproject.New(projectStore, projectStore, artifactStore)
	uiWorkspaceSvc := gatewayuiworkspace.New(uiWorkspaceStore)                                               // Use the Ent-backed uiWorkspaceStore
	uiSvc := gatewayui.New(uiStore, uiWorkspaceSvc, artifactStore, cfg.Interaction.ConversationArtifactPath) // Use the Ent-backed uiStore
	userInteractionSvc := gatewayuserinteraction.New(artifactStore, cfg.Interaction.ConversationArtifactPath)
	workerSvc := gatewayworker.New(projectSvc.AsProjectReader(), projectStore, uiWorkspaceSvc, uiSvc, userInteractionSvc, artifactStore)

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
