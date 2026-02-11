package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

// InitRun initializes a run session. Current implementation is a lightweight mock.
func (s *apiServer) InitRun(_ context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	projectID, userID, repoURL := prepareInitRun(req)
	var (
		sess    initSession
		existed bool
	)
	if projectID == "" {
		if active, ok := getActiveProjectByUser(userID); ok {
			projectID = active.ProjectID
		}
	}
	if projectID != "" {
		sess, existed = getSession(projectID)
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
		sess = initSession{
			SessionID:   projectID,
			ProjectID:   projectID,
			ProjectName: projectName,
			UserID:      userID,
			RepoURL:     repoURL,
			Repo:        repoName,
			RunCtx:      runCtx,
			Running:     false,
			IsActive:    true,
		}
		if runCtx != nil && runCtx.Env != nil {
			runCtx.Env.InitCtx.RepoURL = repoURL
		}
	}
	if repoURL != "" {
		sess.RepoURL = repoURL
	}
	if userID != "" {
		sess.UserID = userID
	}
	sess.SessionID = projectID
	sess.ProjectID = projectID
	if strings.TrimSpace(sess.ProjectName) == "" {
		sess.ProjectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}
	if sess.RunCtx == nil {
		runCtx, err := NewRunContext(sess.Repo, projectID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		if runCtx != nil && runCtx.Env != nil {
			runCtx.Env.InitCtx.SetPurpose(sess.Purpose, sess.RepoURL)
		}
		sess.RunCtx = runCtx
	}

	putSession(sess)
	_, _ = setActiveProjectForUser(sess.UserID, sess.ProjectID)
	persistSessionStore()

	updated, _ := getSession(projectID)

	res := connect.NewResponse(&insightifyv1.InitRunResponse{
		SessionId:      projectID,
		RepoName:       updated.Repo,
		BootstrapRunId: "",
		ProjectId:      updated.ProjectID,
	})
	return res, nil
}
