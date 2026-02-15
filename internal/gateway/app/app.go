package app

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"database/sql"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"insightify/internal/gateway/config"
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/handler/rpc"
	artifactrepo "insightify/internal/gateway/repository/artifact"
	"insightify/internal/gateway/repository/projectstore"
	uirepo "insightify/internal/gateway/repository/ui"
	uiworkspacerepo "insightify/internal/gateway/repository/uiworkspace"
	"insightify/internal/gateway/server"
	gatewayproject "insightify/internal/gateway/service/project"
	gatewayui "insightify/internal/gateway/service/ui"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
	gatewayuserinteraction "insightify/internal/gateway/service/user_interaction"
	gatewayworker "insightify/internal/gateway/service/worker"
)

type App struct {
	server *server.Server
}

type gatewayStores struct {
	ui          uirepo.Store
	artifact    artifactrepo.Store
	uiWorkspace uiworkspacerepo.Store
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
	uiSvc := gatewayui.New(stores.ui, uiWorkspaceSvc)
	userInteractionSvc := gatewayuserinteraction.New()
	workerSvc := gatewayworker.New(projectSvc.AsProjectReader(), uiWorkspaceSvc, uiSvc, userInteractionSvc, stores.artifact)

	projectHandler := rpc.NewProjectHandler(projectSvc)
	runHandler := rpc.NewRunHandler(workerSvc)
	userInteractionHandler := rpc.NewUserInteractionHandler(userInteractionSvc)
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

func initStores(cfg *config.Config) (*gatewayStores, error) {
	if dsn := os.Getenv("PROJECT_STORE_PG_DSN"); dsn != "" {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open db: %w", err)
		}
		stores := &gatewayStores{
			ui:          uirepo.NewPostgresStore(db),
			artifact:    artifactrepo.NewPostgresStore(db),
			uiWorkspace: uiworkspacerepo.NewPostgresStore(db),
		}
		if cfg.Artifact.Enabled {
			s3Cfg := artifactrepo.S3Config{
				Endpoint:  cfg.Artifact.Endpoint,
				Region:    cfg.Artifact.Region,
				AccessKey: cfg.Artifact.AccessKey,
				SecretKey: cfg.Artifact.SecretKey,
				Bucket:    cfg.Artifact.Bucket,
				UseSSL:    cfg.Artifact.UseSSL,
			}
			s3Store, err := artifactrepo.NewS3Store(s3Cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize artifact s3 store: %w", err)
			}
			log.Printf("artifact store: s3 bucket=%s endpoint=%s", s3Cfg.Bucket, s3Cfg.Endpoint)
			stores.artifact = s3Store
		}
		return stores, nil
	}

	return &gatewayStores{
		ui:          uirepo.NewStore(),
		uiWorkspace: uiworkspacerepo.NewMemoryStore(),
	}, nil
}

func (a *App) Start() error {
	return a.server.Start()
}

func (a *App) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}
