package project

import (
	"context"
	"time"

	"insightify/internal/gateway/entity"
)

type Repository interface {
	EnsureLoaded(ctx context.Context)
	Save(ctx context.Context) error
	Get(ctx context.Context, projectID string) (State, bool)
	Put(ctx context.Context, state State) error
	Update(ctx context.Context, projectID string, update func(*State)) (State, bool, error)
	ListByUser(ctx context.Context, userID entity.UserID) ([]State, error)
	GetActiveByUser(ctx context.Context, userID entity.UserID) (State, bool, error)
	SetActiveForUser(ctx context.Context, userID entity.UserID, projectID string) (State, bool, error)
}

type ArtifactRepository interface {
	AddArtifact(ctx context.Context, artifact ProjectArtifact) error
	ListArtifacts(ctx context.Context, projectID string) ([]ProjectArtifact, error)
}

type State struct {
	ProjectID   string        `json:"project_id"`
	ProjectName string        `json:"project_name"`
	UserID      entity.UserID `json:"user_id"`
	Repo        string        `json:"repo"`
	IsActive    bool          `json:"is_active"`
}

type ProjectArtifact struct {
	ID        int       `json:"id"`
	ProjectID string    `json:"project_id"`
	RunID     string    `json:"run_id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}
