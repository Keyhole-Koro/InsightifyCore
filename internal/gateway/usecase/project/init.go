package project

import "strings"

// InitRunInput is normalized input for project init flow.
type InitRunInput struct {
	ProjectID string
	UserID    string
	RepoURL   string
}

// PrepareInitRun normalizes raw init-run parameters.
func PrepareInitRun(userID, repoURL, projectID string) InitRunInput {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		userID = "demo-user"
	}
	return InitRunInput{
		ProjectID: strings.TrimSpace(projectID),
		UserID:    userID,
		RepoURL:   strings.TrimSpace(repoURL),
	}
}
