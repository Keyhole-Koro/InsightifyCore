package project

import (
	"context"
	"time"

	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/artifact"
	entproject "insightify/internal/gateway/ent/project"
	"insightify/internal/gateway/entity"
)

type PostgresStore struct {
	client *ent.Client
}

func NewPostgresStore(client *ent.Client) (*PostgresStore, error) {
	return &PostgresStore{
		client: client,
	}, nil
}

func (s *PostgresStore) EnsureLoaded(_ context.Context) {
	// Schema creation is handled in app.go or via migration tool.
}

func (s *PostgresStore) Save(_ context.Context) error {
	// Ent operations are immediate/transactional.
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, projectID string) (State, bool) {
	p, err := s.client.Project.Query().
		Where(entproject.ID(projectID)).
		Only(ctx)
	if err != nil {
		return State{}, false
	}
	return toState(p), true
}

func (s *PostgresStore) Put(ctx context.Context, state State) error {
	return s.client.Project.Create().
		SetID(state.ProjectID).
		SetName(state.ProjectName).
		SetUserID(state.UserID.String()).
		SetRepo(state.Repo).
		SetIsActive(state.IsActive).
		OnConflictColumns(entproject.FieldID).
		UpdateNewValues().
		Exec(ctx)
}

func (s *PostgresStore) Update(ctx context.Context, projectID string, update func(*State)) (State, bool, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return State{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	p, err := tx.Project.Query().
		Where(entproject.ID(projectID)).
		Only(ctx)
	if err != nil {
		return State{}, false, err
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
		return State{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func (s *PostgresStore) ListByUser(ctx context.Context, userID entity.UserID) ([]State, error) {
	projects, err := s.client.Project.Query().
		Where(entproject.UserID(userID.String())).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]State, 0, len(projects))
	for _, p := range projects {
		out = append(out, toState(p))
	}
	return out, nil
}

func (s *PostgresStore) GetActiveByUser(ctx context.Context, userID entity.UserID) (State, bool, error) {
	p, err := s.client.Project.Query().
		Where(entproject.UserID(userID.String()), entproject.IsActive(true)).
		Only(ctx)
	if err != nil {
		return State{}, false, nil
	}
	return toState(p), true, nil
}

func (s *PostgresStore) SetActiveForUser(ctx context.Context, userID entity.UserID, projectID string) (State, bool, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return State{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Project.Update().
		Where(entproject.UserID(userID.String()), entproject.IsActive(true)).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return State{}, false, err
	}

	p, err := tx.Project.UpdateOneID(projectID).
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		return State{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return State{}, false, err
	}
	return toState(p), true, nil
}

func (s *PostgresStore) AddArtifact(ctx context.Context, a ProjectArtifact) error {
	_, err := s.client.Artifact.Create().
		SetProjectID(a.ProjectID).
		SetRunID(a.RunID).
		SetPath(a.Path).
		SetCreatedAt(time.Now()).
		Save(ctx)
	return err
}

func (s *PostgresStore) ListArtifacts(ctx context.Context, projectID string) ([]ProjectArtifact, error) {
	artifactsEnt, err := s.client.Artifact.Query().
		Where(artifact.ProjectID(projectID)).
		Order(ent.Desc(artifact.FieldCreatedAt)).
		All(ctx)
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
