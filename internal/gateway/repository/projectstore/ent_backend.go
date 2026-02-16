package projectstore

import (
	"context"
	"fmt"
	"time"

	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/artifact"
	"insightify/internal/gateway/ent/project"
)

func (s *Store) ensureEntSchema() error {
	if s.ent == nil {
		return fmt.Errorf("ent client is nil")
	}
	// Auto migration is handled by ent.Schema.Create
	if err := s.ent.Schema.Create(context.Background()); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}
	return nil
}

func (s *Store) getEnt(projectID string) (State, bool) {
	p, err := s.ent.Project.Query().
		Where(project.ID(projectID)).
		Only(context.Background())
	if err != nil {
		return State{}, false
	}
	return entToState(p), true
}

func (s *Store) putEnt(state State) {
	ctx := context.Background()
	// Upsert
	err := s.ent.Project.Create().
		SetID(state.ProjectID).
		SetName(state.ProjectName).
		SetUserID(state.UserID).
		SetRepo(state.Repo).
		SetIsActive(state.IsActive).
		OnConflictColumns(project.FieldID).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		// Log error or handle it
		fmt.Printf("failed to put project: %v\n", err)
	}
}

func (s *Store) updateEnt(projectID string, update func(*State)) (State, bool) {
	ctx := context.Background()
	p, err := s.ent.Project.Query().
		Where(project.ID(projectID)).
		Only(ctx)
	if err != nil {
		return State{}, false
	}

	state := entToState(p)
	update(&state)

	_, err = s.ent.Project.UpdateOneID(projectID).
		SetName(state.ProjectName).
		SetUserID(state.UserID).
		SetRepo(state.Repo).
		SetIsActive(state.IsActive).
		Save(ctx)

	if err != nil {
		return State{}, false
	}

	return state, true
}

func (s *Store) listByUserEnt(userID string) []State {
	projects, err := s.ent.Project.Query().
		Where(project.UserID(userID)).
		All(context.Background())
	if err != nil {
		return nil
	}
	var out []State
	for _, p := range projects {
		out = append(out, entToState(p))
	}
	return out
}

func (s *Store) getActiveByUserEnt(userID string) (State, bool) {
	p, err := s.ent.Project.Query().
		Where(project.UserID(userID), project.IsActive(true)).
		First(context.Background())
	if err != nil {
		return State{}, false
	}
	return entToState(p), true
}

func (s *Store) setActiveForUserEnt(userID, projectID string) (State, bool) {
	ctx := context.Background()
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return State{}, false
	}

	// Deactivate all
	_, err = tx.Project.Update().
		Where(project.UserID(userID)).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		tx.Rollback()
		return State{}, false
	}

	// Activate target
	p, err := tx.Project.UpdateOneID(projectID).
		SetIsActive(true).
		Save(ctx)
	if err != nil {
		tx.Rollback()
		return State{}, false
	}

	if err := tx.Commit(); err != nil {
		return State{}, false
	}

	return entToState(p), true
}

func (s *Store) addArtifactEnt(a ProjectArtifact) error {
	ctx := context.Background()
	err := s.ent.Artifact.Create().
		SetProjectID(a.ProjectID).
		SetRunID(a.RunID).
		SetPath(a.Path).
		SetCreatedAt(time.Now()).
		OnConflictColumns(artifact.FieldRunID, artifact.FieldPath).
		Ignore().
		Exec(ctx)
	return err
}

func (s *Store) listArtifactsEnt(projectID string) ([]ProjectArtifact, error) {
	artifacts, err := s.ent.Artifact.Query().
		Where(artifact.HasProjectWith(project.ID(projectID))).
		Order(ent.Desc(artifact.FieldCreatedAt)).
		All(context.Background())
	if err != nil {
		return nil, err
	}

	var out []ProjectArtifact
	for _, a := range artifacts {
		out = append(out, ProjectArtifact{
			ID:        a.ID,
			ProjectID: projectID, // Known from query context
			RunID:     a.RunID,
			Path:      a.Path,
			CreatedAt: a.CreatedAt,
		})
	}
	return out, nil
}

func entToState(p *ent.Project) State {
	return State{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		UserID:      p.UserID,
		Repo:        p.Repo,
		IsActive:    p.IsActive,
	}
}
