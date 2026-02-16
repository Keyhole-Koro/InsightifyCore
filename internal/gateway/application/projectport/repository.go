package projectport

import (
	"time"

	"insightify/internal/gateway/entity"
)

type ProjectState struct {
	ProjectID   string
	ProjectName string
	UserID      entity.UserID
	Repo        string
	IsActive    bool
}

type ProjectArtifact struct {
	ID        int
	ProjectID string
	RunID     string
	Path      string
	CreatedAt time.Time
}

type Repository interface {
	EnsureLoaded()
	Save() error
	Get(projectID string) (ProjectState, bool)
	Put(state ProjectState)
	Update(projectID string, update func(*ProjectState)) (ProjectState, bool)
	ListByUser(userID entity.UserID) []ProjectState
	GetActiveByUser(userID entity.UserID) (ProjectState, bool)
	SetActiveForUser(userID entity.UserID, projectID string) (ProjectState, bool)
}

type ArtifactRepository interface {
	AddArtifact(artifact ProjectArtifact) error
	ListArtifacts(projectID string) ([]ProjectArtifact, error)
}
