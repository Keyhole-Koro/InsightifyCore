package run

import (
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// StartRunInput is normalized and validated input for starting a run.
type StartRunInput struct {
	ProjectID string
	WorkerKey string
	UserInput string
}

// StartRunDeps defines dependencies required for validating start-run requests.
type StartRunDeps struct {
	ProjectExists          func(projectID string) bool
	EnsureProjectRunContext func(projectID string) error
}

// PrepareStartRun validates start-run parameters and dependency checks.
func PrepareStartRun(projectID, workerKey, userInput string, deps StartRunDeps) (StartRunInput, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return StartRunInput{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	if deps.ProjectExists == nil || !deps.ProjectExists(projectID) {
		return StartRunInput{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	if deps.EnsureProjectRunContext != nil {
		if err := deps.EnsureProjectRunContext(projectID); err != nil {
			return StartRunInput{}, connect.NewError(connect.CodeInternal, err)
		}
	}
	return StartRunInput{
		ProjectID: projectID,
		WorkerKey: strings.TrimSpace(workerKey),
		UserInput: strings.TrimSpace(userInput),
	}, nil
}
