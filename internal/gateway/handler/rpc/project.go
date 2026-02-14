package rpc

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/service/project"

	"connectrpc.com/connect"
)

type ProjectHandler struct {
	svc *project.Service
}

func NewProjectHandler(svc *project.Service) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

func (h *ProjectHandler) ListProjects(ctx context.Context, req *connect.Request[insightifyv1.ListProjectsRequest]) (*connect.Response[insightifyv1.ListProjectsResponse], error) {
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	projects, activeID, err := h.svc.ListProjects(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &insightifyv1.ListProjectsResponse{
		Projects:        make([]*insightifyv1.Project, 0, len(projects)),
		ActiveProjectId: activeID,
	}
	for _, p := range projects {
		out.Projects = append(out.Projects, toProtoProject(p))
	}
	return connect.NewResponse(out), nil
}

func (h *ProjectHandler) CreateProject(ctx context.Context, req *connect.Request[insightifyv1.CreateProjectRequest]) (*connect.Response[insightifyv1.CreateProjectResponse], error) {
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}
	name := strings.TrimSpace(req.Msg.GetName())
	
	p, err := h.svc.CreateProject(ctx, userID, name)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&insightifyv1.CreateProjectResponse{Project: toProtoProject(p)}), nil
}

func (h *ProjectHandler) SelectProject(ctx context.Context, req *connect.Request[insightifyv1.SelectProjectRequest]) (*connect.Response[insightifyv1.SelectProjectResponse], error) {
	userID := strings.TrimSpace(req.Msg.GetUserId())
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	if userID == "" || projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and project_id are required"))
	}

	p, err := h.svc.SelectProject(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&insightifyv1.SelectProjectResponse{Project: toProtoProject(p)}), nil
}

func (h *ProjectHandler) InitRun(ctx context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	userID := strings.TrimSpace(req.Msg.GetUserId())
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	
	p, err := h.svc.InitRun(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&insightifyv1.InitRunResponse{
		RepoName:       p.State.Repo,
		ProjectId:      p.State.ProjectID,
	}), nil
}
