package artifact

import (
	"context"
	"fmt"
	"strings"

	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/artifactfile"
)

type PostgresStore struct {
	client *ent.Client
}

func NewPostgresStore(client *ent.Client) *PostgresStore {
	return &PostgresStore{client: client}
}

func (s *PostgresStore) Put(ctx context.Context, runID, path string, content []byte) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	if s.client == nil {
		return fmt.Errorf("ent client is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if path == "" {
		return fmt.Errorf("path is required")
	}
	if content == nil {
		content = []byte{}
	}
	size := int64(len(content))
	return s.client.ArtifactFile.Create().
		SetRunID(runID).
		SetPath(path).
		SetContent(content).
		SetSize(size).
		OnConflictColumns(artifactfile.FieldRunID, artifactfile.FieldPath).
		UpdateNewValues().
		Exec(ctx)
}

func (s *PostgresStore) Get(ctx context.Context, runID, path string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if s.client == nil {
		return nil, fmt.Errorf("ent client is nil")
	}
	runID = strings.TrimSpace(runID)
	path = strings.TrimSpace(path)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	item, err := s.client.ArtifactFile.Query().
		Where(
			artifactfile.RunID(runID),
			artifactfile.Path(path),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if item == nil {
		return nil, ErrNotFound
	}
	return append([]byte(nil), item.Content...), nil
}

func (s *PostgresStore) List(ctx context.Context, runID string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if s.client == nil {
		return nil, fmt.Errorf("ent client is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	items, err := s.client.ArtifactFile.Query().
		Where(artifactfile.RunID(runID)).
		Order(ent.Asc(artifactfile.FieldPath)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths, nil
}

func (s *PostgresStore) GetURL(ctx context.Context, runID, path string) (string, error) {
	// Postgres store doesn't support URLs (content is stored as BLOB)
	return "", nil
}
