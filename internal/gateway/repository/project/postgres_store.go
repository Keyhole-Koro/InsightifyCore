package project

import (
	"context"
	"fmt"
	"time"

	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/artifact"
	entproject "insightify/internal/gateway/ent/project"
	"insightify/internal/gateway/entity"

	"github.com/hashicorp/golang-lru/v2"
)

type PostgresStore struct {
	client *ent.Client

	// Metadata cache (read-through) keyed by project_id.
	artifactCache *lru.Cache[string, []ProjectArtifact]
}

func NewPostgresStore(client *ent.Client, cache *lru.Cache[string, []ProjectArtifact]) (*PostgresStore, error) {
	return &PostgresStore{
		client:        client,
		artifactCache: cache,
	}, nil
}

func (s *PostgresStore) EnsureLoaded() {
	// Schema creation is handled in app.go or via migration tool.
}

func (s *PostgresStore) Save() error {
	// Ent operations are immediate/transactional.
	return nil
}

func (s *PostgresStore) Get(projectID string) (State, bool) {
	p, err := s.client.Project.Query().
		Where(entproject.ID(projectID)).
		Only(context.Background())
	if err != nil {
		if ent.IsNotFound(err) {
			return State{}, false
		}
		fmt.Printf("failed to get project: %v\n", err)
		return State{}, false
	}
	return toState(p), true
}

func (s *PostgresStore) Put(state State) {
	err := s.client.Project.Create().
		SetID(state.ProjectID).
		SetName(state.ProjectName).
		SetUserID(state.UserID.String()).
		SetRepo(state.Repo).
		SetIsActive(state.IsActive).
		OnConflictColumns(entproject.FieldID).
		UpdateNewValues().
		Exec(context.Background())
	if err != nil {
		fmt.Printf("failed to put project: %v\n", err)
	}
}

func (s *PostgresStore) Update(projectID string, update func(*State)) (State, bool) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return State{}, false
	}
	defer func() { _ = tx.Rollback() }()

	p, err := tx.Project.Query().
		Where(entproject.ID(projectID)).
		Only(ctx)
	if err != nil {
		return State{}, false
	}

	state := toState(p)
	update(&state)

	_, err = tx.Project.UpdateOneID(projectID).
		SetName(state.ProjectName).
		SetUserID(state.UserID.String()).
		SetRepo(state.Repo).
		SetIsActive(state.IsActive).
		Save(ctx)
	if err != nil {
		return State{}, false
	}

	if err := tx.Commit(); err != nil {
		return State{}, false
	}
	return state, true
}

func (s *PostgresStore) ListByUser(userID entity.UserID) []State {
	projects, err := s.client.Project.Query().
		Where(entproject.UserID(userID.String())).
		All(context.Background())
	if err != nil {
		return nil
	}

	out := make([]State, 0, len(projects))
	for _, p := range projects {
		out = append(out, toState(p))
	}
	return out
}

func (s *PostgresStore) GetActiveByUser(userID entity.UserID) (State, bool) {
	p, err := s.client.Project.Query().
		Where(entproject.UserID(userID.String()), entproject.IsActive(true)).
		Only(context.Background())
	if err != nil {
		return State{}, false
	}
	return toState(p), true
}

func (s *PostgresStore) SetActiveForUser(userID entity.UserID, projectID string) (State, bool) {
	ctx := context.Background()
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return State{}, false
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Project.Update().
		Where(entproject.UserID(userID.String()), entproject.IsActive(true)).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return State{}, false
	}

	p, err := tx.Project.UpdateOneID(projectID).
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		return State{}, false
	}

	if err := tx.Commit(); err != nil {
		return State{}, false
	}
	return toState(p), true
}

func (s *PostgresStore) AddArtifact(a ProjectArtifact) error {
	_, err := s.client.Artifact.Create().
		SetProjectID(a.ProjectID).
		SetRunID(a.RunID).
		SetPath(a.Path).
		SetCreatedAt(time.Now()).
		Save(context.Background())
	if err == nil && s.artifactCache != nil {
		s.artifactCache.Remove(a.ProjectID)
	}
	return err
}

func (s *PostgresStore) ListArtifacts(projectID string) ([]ProjectArtifact, error) {
	if s.artifactCache != nil {
		if cached, ok := s.artifactCache.Get(projectID); ok {
			return cached, nil
		}
	}

	artifactsEnt, err := s.client.Artifact.Query().
		Where(artifact.ProjectID(projectID)).
		Order(ent.Desc(artifact.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	out := make([]ProjectArtifact, len(artifactsEnt))
	for i, a := range artifactsEnt {
		out[i] = ProjectArtifact{
			ID:        a.ID,
			ProjectID: a.ProjectID,
			RunID:     a.RunID,
			Path:      a.Path,
			CreatedAt: a.CreatedAt,
		}
	}
	if s.artifactCache != nil {
		s.artifactCache.Add(projectID, out)
	}
	return out, nil
}

func toState(p *ent.Project) State {
	return State{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		UserID:      entity.NormalizeUserID(p.UserID),
		Repo:        p.Repo,
		IsActive:    p.IsActive,
	}
}
