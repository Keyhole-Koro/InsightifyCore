package projectstore

import "strings"

type State struct {
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	UserID      string `json:"user_id"`
	Repo        string `json:"repo"`
	IsActive    bool   `json:"is_active,omitempty"`
}

func normalizeState(state State) State {
	state.ProjectID = strings.TrimSpace(state.ProjectID)
	state.ProjectName = strings.TrimSpace(state.ProjectName)
	state.UserID = strings.TrimSpace(state.UserID)
	state.Repo = strings.TrimSpace(state.Repo)
	if state.ProjectName == "" {
		state.ProjectName = "Project"
	}
	return state
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanState(row rowScanner) (State, bool) {
	var state State
	err := row.Scan(&state.ProjectID, &state.ProjectName, &state.UserID, &state.Repo, &state.IsActive)
	if err != nil {
		return State{}, false
	}
	return normalizeState(state), true
}
