package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/projectstore"

	"connectrpc.com/connect"
)

// InitRun initializes a project run state. Current implementation is a lightweight mock.
func (s *apiServer) InitRun(_ context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	projectID, userID, _ := prepareInitRun(req)
	var (
		sess    projectState
		existed bool
	)
	if projectID == "" {
		if active, ok := getActiveProjectByUser(userID); ok {
			projectID = active.ProjectID
		}
	}
	if projectID != "" {
		sess, existed = getProjectState(projectID)
	}
	if !existed {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			projectID = fmt.Sprintf("project-%d", time.Now().UnixNano())
		}
		repoName := ""
		runCtx, err := NewRunContext(repoName, projectID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		projectName := fmt.Sprintf("Project %d", time.Now().Unix()%100000)
		sess = projectState{
			State: projectstore.State{
				ProjectID:   projectID,
				ProjectName: projectName,
				UserID:      userID,
				Repo:        repoName,
				Running:     false,
				IsActive:    true,
			},
			RunCtx: runCtx,
		}
	}
	if userID != "" {
		sess.UserID = userID
	}
	sess.ProjectID = projectID
	if strings.TrimSpace(sess.ProjectName) == "" {
		sess.ProjectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}
	if sess.RunCtx == nil {
		runCtx, err := NewRunContext(sess.Repo, projectID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		sess.RunCtx = runCtx
	}

	putProjectState(sess)
	_, _ = setActiveProjectForUser(sess.UserID, sess.ProjectID)
	persistProjectStore()

	updated, _ := getProjectState(projectID)

	res := connect.NewResponse(&insightifyv1.InitRunResponse{
		RepoName:       updated.Repo,
		BootstrapRunId: "",
		ProjectId:      updated.ProjectID,
	})
	return res, nil
}
