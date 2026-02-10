package main

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func (s *apiServer) SubmitRunInput(_ context.Context, req *connect.Request[insightifyv1.SubmitRunInputRequest]) (*connect.Response[insightifyv1.SubmitRunInputResponse], error) {
	ensureSessionStoreLoaded()
	sessionID := strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" {
		sessionID = resolveSessionIDFromCookieHeader(req.Header().Get("Cookie"))
	}
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	runID := strings.TrimSpace(req.Msg.GetRunId())
	userInput := strings.TrimSpace(req.Msg.GetInput())
	if userInput == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}

	sess, ok := getSession(sessionID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %s not found", sessionID))
	}
	var err error
	sess, err = ensureSessionRunContext(sessionID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if runID != "" && sess.ActiveRunID != "" && runID != sess.ActiveRunID {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("run %s is not active for session %s", runID, sessionID))
	}

	nextRunID, err := s.launchPlanPipelineRun(sessionID, userInput, false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to submit input: %w", err))
	}
	return connect.NewResponse(&insightifyv1.SubmitRunInputResponse{RunId: nextRunID}), nil
}
