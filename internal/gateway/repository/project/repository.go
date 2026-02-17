package project

import (
	"time"

	"insightify/internal/gateway/entity"
)

type Repository interface {
	EnsureLoaded()
	Save() error
	Get(projectID string) (State, bool)
	Put(state State)
	Update(projectID string, update func(*State)) (State, bool)
	ListByUser(userID entity.UserID) []State
	GetActiveByUser(userID entity.UserID) (State, bool)
	SetActiveForUser(userID entity.UserID, projectID string) (State, bool)
}

type ArtifactRepository interface {
	AddArtifact(artifact ProjectArtifact) error
	ListArtifacts(projectID string) ([]ProjectArtifact, error)
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
