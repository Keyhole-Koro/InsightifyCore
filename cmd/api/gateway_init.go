package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

// InitRun initializes a run session. Current implementation is a lightweight mock.
func (s *apiServer) InitRun(_ context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	sessionID, userID, repoURL := prepareInitRun(req)
	var (
		sess    initSession
		existed bool
	)
	if sessionID != "" {
		sess, existed = getSession(sessionID)
	}
	if !existed {
		sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
		repoName := ""
		runCtx, err := NewRunContext(repoName, sessionID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		sess = initSession{
			SessionID: sessionID,
			UserID:    userID,
			RepoURL:   repoURL,
			Repo:      repoName,
			RunCtx:    runCtx,
			Running:   false,
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
	sess.SessionID = sessionID
	if sess.RunCtx == nil {
		runCtx, err := NewRunContext(sess.Repo, sessionID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		if runCtx != nil && runCtx.Env != nil {
			runCtx.Env.InitCtx.SetPurpose(sess.Purpose, sess.RepoURL)
		}
		sess.RunCtx = runCtx
	}

	putSession(sess)
	persistSessionStore()

	var (
		bootstrapRunID string
		updated        initSession
	)
	current, _ := getSession(sessionID)
	if current.ActiveRunID != "" {
		bootstrapRunID = current.ActiveRunID
		updated = current
	} else {
		var err error
		bootstrapRunID, err = s.launchBootstrapRun(sessionID, "", true)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to bootstrap worker: %w", err))
		}
		updated, _ = updateSession(sessionID, func(cur *initSession) {
			cur.ActiveRunID = bootstrapRunID
		})
	}

	res := connect.NewResponse(&insightifyv1.InitRunResponse{
		SessionId:      sessionID,
		RepoName:       updated.Repo,
		BootstrapRunId: bootstrapRunID,
	})
	res.Header().Add("Set-Cookie", (&http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Local development uses plain HTTP; enable Secure in TLS deployments.
		Secure: false,
	}).String())
	return res, nil
}
