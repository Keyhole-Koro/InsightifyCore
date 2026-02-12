package needinput

import (
	"fmt"
	"strings"

	"connectrpc.com/connect"
)

// SubmitRunInput is normalized and validated user-input submission data.
type SubmitRunInput struct {
	ProjectID     string
	RunID         string
	InteractionID string
	Input         string
}

// PrepareSubmitRunInput validates submit-run-input parameters.
func PrepareSubmitRunInput(projectID, runID, input string, activeRunIDByProject func(projectID string) string) (SubmitRunInput, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return SubmitRunInput{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return SubmitRunInput{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}
	runID = strings.TrimSpace(runID)
	if runID == "" && activeRunIDByProject != nil {
		runID = strings.TrimSpace(activeRunIDByProject(projectID))
	}
	if runID == "" {
		return SubmitRunInput{}, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("run_id is required"))
	}
	return SubmitRunInput{ProjectID: projectID, RunID: runID, Input: input}, nil
}

// PrepareSendMessage validates send-message parameters.
func PrepareSendMessage(projectID, runID, interactionID, input string) (SubmitRunInput, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	if projectID == "" || runID == "" {
		return SubmitRunInput{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and run_id are required"))
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return SubmitRunInput{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}
	return SubmitRunInput{
		ProjectID:     projectID,
		RunID:         runID,
		InteractionID: strings.TrimSpace(interactionID),
		Input:         input,
	}, nil
}
