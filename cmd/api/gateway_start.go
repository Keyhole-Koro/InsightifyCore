package main

import (
	"context"
	"fmt"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

// StartRun executes a single worker and returns the result.
func (s *apiServer) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	sessionID, workerKey, userInput, isBootstrap, err := prepareStartRun(req)
	if err != nil {
		return nil, err
	}

	runID, err := s.launchWorkerRun(sessionID, workerKey, userInput, isBootstrap)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to start %s: %w", workerKey, err))
	}

	return connect.NewResponse(&insightifyv1.StartRunResponse{
		RunId: runID,
	}), nil
}
