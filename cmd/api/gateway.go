package main

import (
	"context"
	"fmt"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/gen/go/insightify/v1/insightifyv1connect"
	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/runner"

	"connectrpc.com/connect"
)

// StartRun executes a single pipeline phase and returns the result.
func (s *apiServer) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	// TODO: Get repo name from params or config. For now, default to a known repo or error if missing.
	repoName := req.Msg.GetParams()["repo_name"]
	if repoName == "" {
		// Fallback for demo purposes if not provided
		repoName = "PoliTopics"
	}

	// Create a new context for this run
	runCtx, err := NewRunContext(repoName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
	}
	defer runCtx.Cleanup()

	key := req.Msg.GetPipelineId()
	if key == "" {
		key = "phase_DAG" // Default phase
	}

	spec, ok := runCtx.Env.Resolver.Get(key)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown phase %s", key))
	}

	// Execute the phase
	out, err := runner.ExecutePhaseWithResult(ctx, spec, runCtx.Env)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("phase %s failed: %w", key, err))
	}

	resp := &insightifyv1.StartRunResponse{}

	if out.ClientView != nil {
		if view, ok := out.ClientView.(*pipelinev1.ClientView); ok {
			resp.ClientView = view
		} else if view, ok := out.ClientView.(pipelinev1.ClientView); ok {
			resp.ClientView = &view
		} else {
			fmt.Printf("DEBUG: ClientView type assertion failed. Got %T\n", out.ClientView)
		}
	}

	return connect.NewResponse(resp), nil
}

// Ensure interface conformance
var _ insightifyv1connect.PipelineServiceHandler = (*apiServer)(nil)
