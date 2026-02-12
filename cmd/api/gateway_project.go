package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/artifact"
	"insightify/internal/gateway/projectstore"

	"connectrpc.com/connect"
)

func toProtoProject(sess projectState) *insightifyv1.Project {
	bootstrapCtx := readBootstrapContext(sess)
	projectID := strings.TrimSpace(sess.ProjectID)
	name := strings.TrimSpace(sess.ProjectName)
	if name == "" {
		name = "Project"
	}
	return &insightifyv1.Project{
		ProjectId: projectID,
		UserId:    strings.TrimSpace(sess.UserID),
		Name:      name,
		RepoUrl:   strings.TrimSpace(bootstrapCtx.RepoURL),
		Purpose:   strings.TrimSpace(bootstrapCtx.Purpose),
		RepoName:  strings.TrimSpace(sess.Repo),
		IsActive:  sess.IsActive,
	}
}

func readBootstrapContext(project projectState) artifact.BootstrapContext {
	if project.RunCtx == nil {
		return artifact.BootstrapContext{}
	}
	path := filepath.Join(project.RunCtx.OutDir, "bootstrap.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return artifact.BootstrapContext{}
	}
	var raw struct {
		BootstrapContext artifact.BootstrapContext `json:"bootstrap_context"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return artifact.BootstrapContext{}
	}
	return raw.BootstrapContext.Normalize()
}

func (s *apiServer) ListProjects(_ context.Context, req *connect.Request[insightifyv1.ListProjectsRequest]) (*connect.Response[insightifyv1.ListProjectsResponse], error) {
	ensureProjectStoreLoaded()
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
	ensureProjectStoreLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	projectName := strings.TrimSpace(req.Msg.GetName())
	if projectName == "" {
		projectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}
	projectID := fmt.Sprintf("project-%d", time.Now().UnixNano())
	runCtx, err := NewRunContext("", projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
	}

	sess := projectState{
		State: projectstore.State{
			ProjectID:   projectID,
			ProjectName: projectName,
			UserID:      userID,
			Repo:        "",
			Running:     false,
			IsActive:    true,
		},
		RunCtx: runCtx,
	}
	putProjectState(sess)
	_, _ = setActiveProjectForUser(userID, projectID)
	persistProjectStore()

	created, _ := getProjectState(projectID)
	return connect.NewResponse(&insightifyv1.CreateProjectResponse{
		Project: toProtoProject(created),
	}), nil
}

func (s *apiServer) SelectProject(_ context.Context, req *connect.Request[insightifyv1.SelectProjectRequest]) (*connect.Response[insightifyv1.SelectProjectResponse], error) {
	ensureProjectStoreLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	if userID == "" || projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and project_id are required"))
	}

	sess, ok := getProjectState(projectID)
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
	persistProjectStore()

	return connect.NewResponse(&insightifyv1.SelectProjectResponse{
		Project: toProtoProject(selected),
	}), nil
}
