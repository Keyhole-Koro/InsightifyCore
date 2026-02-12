package project

import (
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
)

type Session struct {
	ProjectID   string
	ProjectName string
	UserID      string
	Repo        string
	IsActive    bool
	RunCtx      any
}

type Deps struct {
	EnsureLoaded       func()
	Persist            func()
	GetState           func(projectID string) (Session, bool)
	PutState           func(sess Session)
	ListByUser         func(userID string) []Session
	GetActiveByUser    func(userID string) (Session, bool)
	SetActiveForUser   func(userID, projectID string) (Session, bool)
	NewRunContext      func(repo, projectID string) (any, error)
	HasRequiredWorkers func(runCtx any) bool
}

func List(userID string, deps Deps) ([]Session, error) {
	if deps.EnsureLoaded != nil {
		deps.EnsureLoaded()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}
	if deps.ListByUser == nil {
		return nil, nil
	}
	return deps.ListByUser(userID), nil
}

func Create(userID, projectName string, now time.Time, deps Deps) (Session, error) {
	if deps.EnsureLoaded != nil {
		deps.EnsureLoaded()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Session{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		projectName = fmt.Sprintf("Project %d", now.Unix()%100000)
	}
	projectID := fmt.Sprintf("project-%d", now.UnixNano())

	var runCtx any
	if deps.NewRunContext != nil {
		ctx, err := deps.NewRunContext("", projectID)
		if err != nil {
			return Session{}, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		runCtx = ctx
	}

	sess := Session{
		ProjectID:   projectID,
		ProjectName: projectName,
		UserID:      userID,
		Repo:        "",
		IsActive:    true,
		RunCtx:      runCtx,
	}
	if deps.PutState != nil {
		deps.PutState(sess)
	}
	if deps.SetActiveForUser != nil {
		_, _ = deps.SetActiveForUser(userID, projectID)
	}
	if deps.Persist != nil {
		deps.Persist()
	}
	if deps.GetState != nil {
		if created, ok := deps.GetState(projectID); ok {
			return created, nil
		}
	}
	return sess, nil
}

func Select(userID, projectID string, deps Deps) (Session, error) {
	if deps.EnsureLoaded != nil {
		deps.EnsureLoaded()
	}
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return Session{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and project_id are required"))
	}
	if deps.GetState == nil {
		return Session{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	sess, ok := deps.GetState(projectID)
	if !ok {
		return Session{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	if strings.TrimSpace(sess.UserID) != userID {
		return Session{}, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("project %s does not belong to user %s", projectID, userID))
	}
	if deps.SetActiveForUser == nil {
		return Session{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	selected, ok := deps.SetActiveForUser(userID, projectID)
	if !ok {
		return Session{}, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	if deps.Persist != nil {
		deps.Persist()
	}
	return selected, nil
}

func InitRun(in InitRunInput, now time.Time, deps Deps) (Session, error) {
	if deps.EnsureLoaded != nil {
		deps.EnsureLoaded()
	}
	projectID := strings.TrimSpace(in.ProjectID)
	userID := strings.TrimSpace(in.UserID)
	if userID == "" {
		userID = "demo-user"
	}

	var (
		sess    Session
		existed bool
	)
	if projectID == "" && deps.GetActiveByUser != nil {
		if active, ok := deps.GetActiveByUser(userID); ok {
			projectID = active.ProjectID
		}
	}
	if projectID != "" && deps.GetState != nil {
		sess, existed = deps.GetState(projectID)
	}
	if !existed {
		if projectID == "" {
			projectID = fmt.Sprintf("project-%d", now.UnixNano())
		}
		projectName := fmt.Sprintf("Project %d", now.Unix()%100000)
		var runCtx any
		if deps.NewRunContext != nil {
			ctx, err := deps.NewRunContext("", projectID)
			if err != nil {
				return Session{}, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
			}
			runCtx = ctx
		}
		sess = Session{
			ProjectID:   projectID,
			ProjectName: projectName,
			UserID:      userID,
			Repo:        "",
			IsActive:    true,
			RunCtx:      runCtx,
		}
	}
	if userID != "" {
		sess.UserID = userID
	}
	sess.ProjectID = projectID
	if strings.TrimSpace(sess.ProjectName) == "" {
		sess.ProjectName = fmt.Sprintf("Project %d", now.Unix()%100000)
	}
	if (deps.HasRequiredWorkers == nil || !deps.HasRequiredWorkers(sess.RunCtx)) && deps.NewRunContext != nil {
		ctx, err := deps.NewRunContext(sess.Repo, projectID)
		if err != nil {
			return Session{}, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		sess.RunCtx = ctx
	}

	if deps.PutState != nil {
		deps.PutState(sess)
	}
	if deps.SetActiveForUser != nil {
		_, _ = deps.SetActiveForUser(sess.UserID, sess.ProjectID)
	}
	if deps.Persist != nil {
		deps.Persist()
	}
	if deps.GetState != nil {
		if updated, ok := deps.GetState(projectID); ok {
			return updated, nil
		}
	}
	return sess, nil
}
