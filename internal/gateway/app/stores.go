package app

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	artifactcache "insightify/internal/cache/artifact"
	uicache "insightify/internal/cache/ui"
	uiworkspacecache "insightify/internal/cache/uiworkspace"
	"insightify/internal/gateway/config"
	"insightify/internal/gateway/ent"
	artifactrepo "insightify/internal/gateway/repository/artifact"
	uirepo "insightify/internal/gateway/repository/ui"
	uiworkspacerepo "insightify/internal/gateway/repository/uiworkspace"
)

type gatewayStores struct {
	ui          uirepo.Store
	artifact    artifactrepo.Store
	uiWorkspace uiworkspacerepo.Store
}

func initStores(cfg *config.Config) (*gatewayStores, error) {
	s3Factory := newArtifactS3StoreFactory(cfg)

	if dsn := strings.TrimSpace(cfg.DatabaseURL); dsn != "" {
		return initPostgresStores(dsn, cfg, s3Factory)
	}
	return initInMemoryStores(cfg, s3Factory)
}

func newArtifactS3StoreFactory(cfg *config.Config) func() (artifactrepo.Store, error) {
	return func() (artifactrepo.Store, error) {
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
		return s3Store, nil
	}
}

func initPostgresStores(dsn string, cfg *config.Config, s3Factory func() (artifactrepo.Store, error)) (*gatewayStores, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	// Initialize Ent client
	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	stores := &gatewayStores{
		ui:          uicache.NewCachedStore(uirepo.NewPostgresStore(client), uicache.DefaultCacheConfig()),
		artifact:    artifactrepo.NewPostgresStore(client),
		uiWorkspace: uiworkspacecache.NewCachedStore(uiworkspacerepo.NewPostgresStore(client), uiworkspacecache.DefaultCacheConfig()),
	}
	artifactStore, err := chooseArtifactStore(cfg, stores.artifact, "postgres", s3Factory)
	if err != nil {
		return nil, err
	}
	stores.artifact = artifactStore
	return stores, nil
}

func initInMemoryStores(cfg *config.Config, s3Factory func() (artifactrepo.Store, error)) (*gatewayStores, error) {
	artifactStore, err := chooseArtifactStore(cfg, artifactcache.NewMemoryStore(), "in-memory", s3Factory)
	if err != nil {
		return nil, err
	}
	return &gatewayStores{
		ui:          uicache.NewCachedStore(uicache.NewMemoryStore(), uicache.DefaultCacheConfig()),
		artifact:    artifactStore,
		uiWorkspace: uiworkspacecache.NewCachedStore(uiworkspacecache.NewMemoryStore(), uiworkspacecache.DefaultCacheConfig()),
	}, nil
}

func chooseArtifactStore(
	cfg *config.Config,
	fallback artifactrepo.Store,
	fallbackLabel string,
	s3Factory func() (artifactrepo.Store, error),
) (artifactrepo.Store, error) {
	var origin artifactrepo.Store
	if cfg.Artifact.CanUseS3() {
		s3Store, err := s3Factory()
		if err != nil {
			return nil, err
		}
		origin = s3Store
	} else {
		if cfg.Artifact.Enabled {
			log.Printf("artifact store: using %s fallback (s3 config incomplete)", fallbackLabel)
		}
		origin = fallback
	}
	if origin == nil {
		return nil, fmt.Errorf("artifact origin store is nil")
	}
	return artifactcache.NewCachedStore(origin, artifactcache.DefaultCacheConfig()), nil
}
