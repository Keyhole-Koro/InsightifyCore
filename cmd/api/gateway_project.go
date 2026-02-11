package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

func toProtoProject(sess initSession) *insightifyv1.Project {
	projectID := strings.TrimSpace(sess.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(sess.SessionID)
	}
	name := strings.TrimSpace(sess.ProjectName)
	if name == "" {
		name = "Project"
	}
	return &insightifyv1.Project{
		ProjectId: projectID,
		UserId:    strings.TrimSpace(sess.UserID),
		Name:      name,
		RepoUrl:   strings.TrimSpace(sess.RepoURL),
		Purpose:   strings.TrimSpace(sess.Purpose),
		RepoName:  strings.TrimSpace(sess.Repo),
		IsActive:  sess.IsActive,
	}
}

func (s *apiServer) ListProjects(_ context.Context, req *connect.Request[insightifyv1.ListProjectsRequest]) (*connect.Response[insightifyv1.ListProjectsResponse], error) {
	ensureSessionStoreLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	projects := listProjectsByUser(userID)
	sort.Slice(projects, func(i, j int) bool {
		return strings.TrimSpace(projects[i].ProjectID) < strings.TrimSpace(projects[j].ProjectID)
	})

	out := &insightifyv1.ListProjectsResponse{
		Projects: make([]*insightifyv1.Project, 0, len(projects)),
	}
	for _, p := range projects {
		protoProject := toProtoProject(p)
		out.Projects = append(out.Projects, protoProject)
		if protoProject.GetIsActive() && out.GetActiveProjectId() == "" {
			out.ActiveProjectId = protoProject.GetProjectId()
		}
	}
	return connect.NewResponse(out), nil
}

func (s *apiServer) CreateProject(_ context.Context, req *connect.Request[insightifyv1.CreateProjectRequest]) (*connect.Response[insightifyv1.CreateProjectResponse], error) {
	ensureSessionStoreLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	projectName := strings.TrimSpace(req.Msg.GetName())
	if projectName == "" {
		projectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}
	repoURL := strings.TrimSpace(req.Msg.GetRepoUrl())

	projectID := fmt.Sprintf("project-%d", time.Now().UnixNano())
	runCtx, err := NewRunContext("", projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
	}
	if runCtx != nil && runCtx.Env != nil {
		runCtx.Env.InitCtx.RepoURL = repoURL
	}

	sess := initSession{
		SessionID:   projectID,
		ProjectID:   projectID,
		ProjectName: projectName,
		UserID:      userID,
		RepoURL:     repoURL,
		Repo:        "",
		RunCtx:      runCtx,
		Running:     false,
		IsActive:    true,
	}
	putSession(sess)
	_, _ = setActiveProjectForUser(userID, projectID)
	persistSessionStore()

	created, _ := getSession(projectID)
	return connect.NewResponse(&insightifyv1.CreateProjectResponse{
		Project: toProtoProject(created),
	}), nil
}

func (s *apiServer) SelectProject(_ context.Context, req *connect.Request[insightifyv1.SelectProjectRequest]) (*connect.Response[insightifyv1.SelectProjectResponse], error) {
	ensureSessionStoreLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	if userID == "" || projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and project_id are required"))
	}

	sess, ok := getSession(projectID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	if strings.TrimSpace(sess.UserID) != userID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("project %s does not belong to user %s", projectID, userID))
	}

	selected, ok := setActiveProjectForUser(userID, projectID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	persistSessionStore()

	return connect.NewResponse(&insightifyv1.SelectProjectResponse{
		Project: toProtoProject(selected),
	}), nil
}
