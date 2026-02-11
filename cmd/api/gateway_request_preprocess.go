package main

import (
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func prepareInitRun(req *connect.Request[insightifyv1.InitRunRequest]) (projectID, userID, repoURL string) {
	ensureSessionStoreLoaded()
	userID = strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		userID = "demo-user"
	}
	repoURL = strings.TrimSpace(req.Msg.GetRepoUrl())
	projectID = strings.TrimSpace(req.Msg.GetProjectId())
	return projectID, userID, repoURL
}

func prepareStartRun(req *connect.Request[insightifyv1.StartRunRequest]) (projectID, workerKey, userInput string, err error) {
	ensureSessionStoreLoaded()
	workerKey = req.Msg.GetPipelineId()
	projectID = strings.TrimSpace(req.Msg.GetProjectId())
	if projectID == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	if _, ok := getSession(projectID); !ok {
		return "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	if _, ensureErr := ensureSessionRunContext(projectID); ensureErr != nil {
		return "", "", "", connect.NewError(connect.CodeInternal, ensureErr)
	}
	userInput = strings.TrimSpace(req.Msg.GetParams()["user_input"])
	return projectID, workerKey, userInput, nil
}

func prepareNeedUserInput(req *connect.Request[insightifyv1.SubmitRunInputRequest]) (projectID, runID, userInput string, err error) {
	ensureSessionStoreLoaded()
	projectID = strings.TrimSpace(req.Msg.GetProjectId())
	if projectID == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	userInput = strings.TrimSpace(req.Msg.GetInput())
	if userInput == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}

	sess, ok := getSession(projectID)
	if !ok {
		return "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	runID = strings.TrimSpace(req.Msg.GetRunId())
	if runID == "" {
		runID = strings.TrimSpace(sess.ActiveRunID)
	}
	if runID == "" {
		return "", "", "", connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("run_id is required"))
	}
	return projectID, runID, userInput, nil
}

func prepareSendMessage(req *connect.Request[insightifyv1.SendMessageRequest]) (projectID, runID, interactionID, input string, err error) {
	ensureSessionStoreLoaded()
	projectID = strings.TrimSpace(req.Msg.GetProjectId())
	runID = strings.TrimSpace(req.Msg.GetRunId())
	interactionID = strings.TrimSpace(req.Msg.GetInteractionId())
	input = strings.TrimSpace(req.Msg.GetInput())
	if projectID == "" || runID == "" {
		return "", "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and run_id are required"))
	}
	if input == "" {
		return "", "", "", "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}
	return projectID, runID, interactionID, input, nil
}
