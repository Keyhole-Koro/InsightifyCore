package main

import (
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func prepareInitRun(req *connect.Request[insightifyv1.InitRunRequest]) (sessionID, userID, repoURL string) {
	ensureSessionStoreLoaded()
	userID = strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		userID = "demo-user"
	}
	repoURL = strings.TrimSpace(req.Msg.GetRepoUrl())
	// Resolve session from cookie to keep frontend session/cookie alignment on refresh.
	sessionID = resolveSessionIDFromCookieHeader(req.Header().Get("Cookie"))
	return sessionID, userID, repoURL
}

func prepareStartRun(req *connect.Request[insightifyv1.StartRunRequest]) (sessionID, workerKey, userInput string, err error) {
	ensureSessionStoreLoaded()
	workerKey = req.Msg.GetPipelineId()
	// Resolve session from request first, then cookie fallback to support browser reconnect flows.
	sessionID = resolveSessionID(req)
	if sessionID == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required (request field or cookie)"))
	}
	if _, ok := getSession(sessionID); !ok {
		return "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("session %s not found", sessionID))
	}
	if _, ensureErr := ensureSessionRunContext(sessionID); ensureErr != nil {
		return "", "", "", connect.NewError(connect.CodeInternal, ensureErr)
	}
	userInput = strings.TrimSpace(req.Msg.GetParams()["user_input"])
	return sessionID, workerKey, userInput, nil
}

func prepareNeedUserInput(req *connect.Request[insightifyv1.SubmitRunInputRequest]) (sessionID, runID, userInput string, err error) {
	ensureSessionStoreLoaded()
	sessionID = strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" {
		// Resolve session from cookie fallback when frontend omits session_id.
		sessionID = resolveSessionIDFromCookieHeader(req.Header().Get("Cookie"))
	}
	if sessionID == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	userInput = strings.TrimSpace(req.Msg.GetInput())
	if userInput == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}

	sess, ok := getSession(sessionID)
	if !ok {
		return "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("session %s not found", sessionID))
	}
	runID = strings.TrimSpace(req.Msg.GetRunId())
	if runID == "" {
		runID = strings.TrimSpace(sess.ActiveRunID)
	}
	if runID == "" {
		return "", "", "", connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("run_id is required"))
	}
	return sessionID, runID, userInput, nil
}

func prepareSendMessage(req *connect.Request[insightifyv1.SendMessageRequest]) (sessionID, runID, interactionID, input string, err error) {
	ensureSessionStoreLoaded()
	sessionID = strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" {
		// Resolve session from cookie fallback when chat send omits session_id.
		sessionID = resolveSessionIDFromCookieHeader(req.Header().Get("Cookie"))
	}
	runID = strings.TrimSpace(req.Msg.GetRunId())
	interactionID = strings.TrimSpace(req.Msg.GetInteractionId())
	input = strings.TrimSpace(req.Msg.GetInput())
	if sessionID == "" || runID == "" {
		return "", "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id and run_id are required"))
	}
	if input == "" {
		return "", "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}
	return sessionID, runID, interactionID, input, nil
}
