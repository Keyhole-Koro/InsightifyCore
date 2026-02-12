package projectstore

import (
	"database/sql"
	"os"
	"strings"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	path string
	db   *sql.DB

	loadOnce sync.Once
	mu       sync.RWMutex
	byID     map[string]State

	schemaOnce sync.Once
	schemaErr  error
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
	return &Store{db: db}, nil
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
	if s.db != nil {
		return s.getDB(projectID)
	}
	return s.getFile(projectID)
}

func (s *Store) Put(state State) {
	if s == nil {
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
	if s.db != nil {
		return s.updateDB(projectID, update)
	}
	return s.updateFile(projectID, update)
}

func (s *Store) ListByUser(userID string) []State {
	if s == nil {
		return nil
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
	if s.db != nil {
		return s.getActiveByUserDB(userID)
	}
	return s.getActiveByUserFile(userID)
}

func (s *Store) SetActiveForUser(userID, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.db != nil {
		return s.setActiveForUserDB(userID, projectID)
	}
	return s.setActiveForUserFile(userID, projectID)
}
