package projectstore

import (
	"database/sql"
	"os"
	"strings"
	"sync"

	"insightify/internal/gateway/ent"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/hashicorp/golang-lru/v2"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	path string
	db   *sql.DB
	ent  *ent.Client

	loadOnce sync.Once
	mu       sync.RWMutex
	byID     map[string]State

	schemaOnce sync.Once
	schemaErr  error

	artifactCache *lru.Cache[string, []ProjectArtifact]
}

func New(path string) *Store {
	return &Store{
		path: path,
		byID: make(map[string]State),
	}
}

func NewPostgres(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", strings.TrimSpace(dsn))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Initialize cache with 1024 entries
	cache, err := lru.New[string, []ProjectArtifact](1024)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	// Initialize Ent client
	drv := entsql.OpenDB("pgx", db)
	client := ent.NewClient(ent.Driver(drv))

	return &Store{
		db:            db,
		ent:           client,
		artifactCache: cache,
	}, nil
}

func NewFromEnv(path string) *Store {
	dsn := strings.TrimSpace(os.Getenv("PROJECT_STORE_PG_DSN"))
	if dsn == "" {
		return New(path)
	}
	s, err := NewPostgres(dsn)
	if err != nil {
		return New(path)
	}
	return s
}

func (s *Store) EnsureLoaded() {
	if s == nil {
		return
	}
	if s.ent != nil {
		_ = s.ensureEntSchema()
		return
	}
	if s.db != nil {
		_ = s.ensureSchema()
		return
	}
	s.ensureLoadedFile()
}

func (s *Store) Save() {
	if s == nil || s.db != nil {
		return
	}
	s.saveFile()
}

func (s *Store) Get(projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.getEnt(projectID)
	}
	if s.db != nil {
		return s.getDB(projectID)
	}
	return s.getFile(projectID)
}

func (s *Store) Put(state State) {
	if s == nil {
		return
	}
	if s.ent != nil {
		s.putEnt(state)
		return
	}
	if s.db != nil {
		s.putDB(state)
		return
	}
	s.putFile(state)
}

func (s *Store) Update(projectID string, update func(*State)) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.updateEnt(projectID, update)
	}
	if s.db != nil {
		return s.updateDB(projectID, update)
	}
	return s.updateFile(projectID, update)
}

func (s *Store) ListByUser(userID string) []State {
	if s == nil {
		return nil
	}
	if s.ent != nil {
		return s.listByUserEnt(userID)
	}
	if s.db != nil {
		return s.listByUserDB(userID)
	}
	return s.listByUserFile(userID)
}

func (s *Store) GetActiveByUser(userID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.getActiveByUserEnt(userID)
	}
	if s.db != nil {
		return s.getActiveByUserDB(userID)
	}
	return s.getActiveByUserFile(userID)
}

func (s *Store) SetActiveForUser(userID, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.setActiveForUserEnt(userID, projectID)
	}
	if s.db != nil {
		return s.setActiveForUserDB(userID, projectID)
	}
	return s.setActiveForUserFile(userID, projectID)
}

func (s *Store) AddArtifact(artifact ProjectArtifact) error {
	if s == nil {
		return nil
	}
	if s.ent != nil {
		err := s.addArtifactEnt(artifact)
		if err == nil && s.artifactCache != nil {
			s.artifactCache.Remove(artifact.ProjectID)
		}
		return err
	}
	if s.db != nil {
		err := s.addArtifactDB(artifact)
		if err == nil && s.artifactCache != nil {
			s.artifactCache.Remove(artifact.ProjectID)
		}
		return err
	}
	return s.addArtifactFile(artifact)
}

func (s *Store) ListArtifacts(projectID string) ([]ProjectArtifact, error) {
	if s == nil {
		return nil, nil
	}
	
	// Check cache first (Read-Through)
	if s.artifactCache != nil {
		if cached, ok := s.artifactCache.Get(projectID); ok {
			return cached, nil
		}
	}

	if s.ent != nil {
		artifacts, err := s.listArtifactsEnt(projectID)
		if err != nil {
			return nil, err
		}
		if s.artifactCache != nil {
			s.artifactCache.Add(projectID, artifacts)
		}
		return artifacts, nil
	}

	if s.db != nil {
		artifacts, err := s.listArtifactsDB(projectID)
		if err != nil {
			return nil, err
		}

		if s.artifactCache != nil {
			s.artifactCache.Add(projectID, artifacts)
		}
		return artifacts, nil
	}
	return s.listArtifactsFile(projectID)
}
