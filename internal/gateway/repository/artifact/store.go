package artifact

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Store defines operations for persisting run artifacts.
type Store interface {
	Put(ctx context.Context, runID, path string, content []byte) error
	Get(ctx context.Context, runID, path string) ([]byte, error)
	GetURL(ctx context.Context, runID, path string) (string, error)
	List(ctx context.Context, runID string) ([]string, error)
}

type PostgresStore struct {
	db         *sql.DB
	schemaOnce sync.Once
	schemaErr  error
}

var ErrNotFound = errors.New("artifact not found")

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) ensureSchema() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("db is nil")
	}
	s.schemaOnce.Do(func() {
		_, s.schemaErr = s.db.Exec(`
CREATE TABLE IF NOT EXISTS artifact_files (
    id SERIAL PRIMARY KEY,
    run_id TEXT NOT NULL,
    path TEXT NOT NULL,
    content BYTEA NOT NULL DEFAULT ''::bytea,
    size BIGINT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(run_id, path)
);
CREATE INDEX IF NOT EXISTS idx_artifacts_run_id ON artifact_files(run_id);
`)
	})
	return s.schemaErr
}

func (s *PostgresStore) Put(ctx context.Context, runID, path string, content []byte) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if err := s.ensureSchema(); err != nil {
		return err
	}
	if content == nil {
		content = []byte{}
	}
	size := int64(len(content))
	_, err := s.db.ExecContext(ctx, `
INSERT INTO artifact_files (run_id, path, content, size, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, path)
DO UPDATE SET content=EXCLUDED.content, size=EXCLUDED.size, updated_at=EXCLUDED.updated_at
`, runID, path, content, size, time.Now())
	return err
}

func (s *PostgresStore) Get(ctx context.Context, runID, path string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	var content []byte
	err := s.db.QueryRowContext(ctx, `SELECT content FROM artifact_files WHERE run_id=$1 AND path=$2`, runID, path).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return content, err
}

func (s *PostgresStore) List(ctx context.Context, runID string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if err := s.ensureSchema(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM artifact_files WHERE run_id=$1 ORDER BY path`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}

func (s *PostgresStore) GetURL(ctx context.Context, runID, path string) (string, error) {
	// Postgres store doesn't support URLs (content is stored as BLOB)
	return "", nil
}
