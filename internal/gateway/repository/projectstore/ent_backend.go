package projectstore

import (
	"context"
	"fmt"
	"time"

	"insightify/internal/gateway/ent"
	"insightify/internal/gateway/ent/artifact"
	"insightify/internal/gateway/ent/project"
)

// Ent Backend Implementation

func (s *Store) ensureEntSchema() error {
	// Schema creation is handled in app.go or via migration tool
	return nil
}

func (s *Store) getEnt(projectID string) (State, bool) {
	p, err := s.ent.Project.Query().
		Where(project.ID(projectID)).
		Only(context.Background())
	
	if err != nil {
		if ent.IsNotFound(err) {
			return State{}, false
		}
		// Log error?
		fmt.Printf("failed to get project: %v\n", err)
		return State{}, false
	}

	return State{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		UserID:      p.UserID,
		Repo:        p.Repo,
		IsActive:    p.IsActive,
	}, true
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
		fmt.Printf("failed to put project: %v\n", err)
	}
}

func (s *Store) updateEnt(projectID string, update func(*State)) (State, bool) {
	ctx := context.Background()
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return State{}, false
	}
	defer func() { _ = tx.Rollback() }()

	p, err := tx.Project.Query().
		Where(project.ID(projectID)).
		// Locking handled by transaction isolation or optimistic locking if needed
		Only(ctx)
	
	if err != nil {
		return State{}, false
	}

	state := State{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		UserID:      p.UserID,
		Repo:        p.Repo,
		IsActive:    p.IsActive,
	}

	update(&state)

	_, err = tx.Project.UpdateOneID(projectID).
		SetName(state.ProjectName).
		SetUserID(state.UserID).
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

func (s *Store) listByUserEnt(userID string) []State {
	projects, err := s.ent.Project.Query().
		Where(project.UserID(userID)).
		All(context.Background())
	
	if err != nil {
		return nil
	}

	var out []State
	for _, p := range projects {
		out = append(out, State{
			ProjectID:   p.ID,
			ProjectName: p.Name,
			UserID:      p.UserID,
			Repo:        p.Repo,
			IsActive:    p.IsActive,
		})
	}
	return out
}

func (s *Store) getActiveByUserEnt(userID string) (State, bool) {
	p, err := s.ent.Project.Query().
		Where(project.UserID(userID), project.IsActive(true)).
		Only(context.Background())
	
	if err != nil {
		return State{}, false
	}
	
	return State{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		UserID:      p.UserID,
		Repo:        p.Repo,
		IsActive:    p.IsActive,
	}, true
}

func (s *Store) setActiveForUserEnt(userID, projectID string) (State, bool) {
	ctx := context.Background()
	tx, err := s.ent.Tx(ctx)
	if err != nil {
		return State{}, false
	}
	defer func() { _ = tx.Rollback() }()

	// Deactivate others
	_, err = tx.Project.Update().
		Where(project.UserID(userID), project.IsActive(true)).
		SetIsActive(false).
		Save(ctx)
	if err != nil {
		return State{}, false
	}

	// Activate target and get it
	p, err := tx.Project.UpdateOneID(projectID).
		SetIsActive(true).
		Save(ctx)
	
	if err != nil {
		return State{}, false
	}

	if err := tx.Commit(); err != nil {
		return State{}, false
	}

	return State{
		ProjectID:   p.ID,
		ProjectName: p.Name,
		UserID:      p.UserID,
		Repo:        p.Repo,
		IsActive:    p.IsActive,
	}, true
}

// Artifacts

func (s *Store) addArtifactEnt(a ProjectArtifact) error {
	_, err := s.ent.Artifact.Create().
		SetProjectID(a.ProjectID).
		SetRunID(a.RunID).
		SetPath(a.Path).
		SetCreatedAt(time.Now()).
		Save(context.Background())
	return err
}

func (s *Store) listArtifactsEnt(projectID string) ([]ProjectArtifact, error) {
	artifacts, err := s.ent.Artifact.Query().
		Where(artifact.ProjectID(projectID)).
		Order(ent.Desc(artifact.FieldCreatedAt)).
		All(context.Background())
	
	if err != nil {
		return nil, err
	}

	out := make([]ProjectArtifact, len(artifacts))
	for i, a := range artifacts {
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
