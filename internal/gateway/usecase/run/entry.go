package run

import (
	"fmt"

	"connectrpc.com/connect"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

// StartRunEntryDeps defines required dependencies for StartRun entry handling.
type StartRunEntryDeps struct {
	PrepareDeps StartRunDeps
	LaunchRun   func(projectID, workerKey, userInput string) (string, error)
}

// StartRunEntry handles request preprocessing and delegation for StartRun.
type StartRunEntry struct {
	deps StartRunEntryDeps
}

func NewStartRunEntry(deps StartRunEntryDeps) *StartRunEntry {
	return &StartRunEntry{deps: deps}
}

func (e *StartRunEntry) Handle(req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	if req == nil || req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("request is required"))
	}
	in, err := PrepareStartRun(
		req.Msg.GetProjectId(),
		req.Msg.GetPipelineId(),
		req.Msg.GetParams()["user_input"],
		e.deps.PrepareDeps,
	)
	if err != nil {
		return nil, err
	}
	if e.deps.LaunchRun == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("launch dependency is not configured"))
	}
	runID, err := e.deps.LaunchRun(in.ProjectID, in.WorkerKey, in.UserInput)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start %s: %w", in.WorkerKey, err))
	}
	return connect.NewResponse(&insightifyv1.StartRunResponse{RunId: runID}), nil
}
