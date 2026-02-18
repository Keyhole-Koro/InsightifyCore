package ui

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	artifactrepo "insightify/internal/gateway/repository/artifact"
	uirepo "insightify/internal/gateway/repository/ui"
	uiworkspacerepo "insightify/internal/gateway/repository/uiworkspace"
	gatewayrestore "insightify/internal/gateway/service/restore"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
)

// Service provides UI node state operations for gateway services.
type Service struct {
	store                    uirepo.Store
	workspaces               *gatewayuiworkspace.Service
	restore                  *gatewayrestore.Service
	artifact                 artifactrepo.Store
	conversationArtifactPath string
}

func New(store uirepo.Store, workspaces *gatewayuiworkspace.Service, artifact artifactrepo.Store, conversationArtifactPath string) *Service {
	path := strings.TrimSpace(conversationArtifactPath)
	if path == "" {
		path = "interaction/conversation_history.json"
	}
	return &Service{
		store:                    store,
		workspaces:               workspaces,
		restore:                  gatewayrestore.New(store, workspaces),
		artifact:                 artifact,
		conversationArtifactPath: path,
	}
}

func (s *Service) GetDocument(ctx context.Context, req *insightifyv1.GetUiDocumentRequest) (*insightifyv1.GetUiDocumentResponse, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("ui service is not available")
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	doc, err := s.store.GetDocument(ctx, runID)
	if err != nil {
		return nil, err
	}
	doc = s.withConversationHistory(ctx, runID, doc)
	return &insightifyv1.GetUiDocumentResponse{Document: doc}, nil
}

func (s *Service) ApplyOps(ctx context.Context, req *insightifyv1.ApplyUiOpsRequest) (*insightifyv1.ApplyUiOpsResponse, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("ui service is not available")
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	doc, conflict, err := s.store.ApplyOps(ctx, runID, req.GetBaseVersion(), req.GetOps())
	if err != nil {
		return nil, err
	}

	res := &insightifyv1.ApplyUiOpsResponse{
		Document:       doc,
		Conflict:       conflict,
		CurrentVersion: doc.GetVersion(),
	}
	if conflict {
		res.ConflictMessage = "base_version does not match current document version"
	}
	return res, nil
}

func (s *Service) Restore(ctx context.Context, req *insightifyv1.RestoreUiRequest) (*insightifyv1.RestoreUiResponse, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("ui service is not available")
	}
	if s.restore == nil {
		return nil, fmt.Errorf("ui restore service is not available")
	}
	projectID := strings.TrimSpace(req.GetProjectId())
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}

	result, err := s.restore.ResolveProjectTabDocument(ctx, projectID, req.GetTabId())
	if err != nil {
		return nil, err
	}
	if result.Found && result.Document != nil {
		result.Document = s.withConversationHistory(ctx, result.RunID, result.Document)
	}
	return result.ToRestoreProtoResponse(), nil
}

func (s *Service) GetWorkspace(_ context.Context, req *insightifyv1.GetUiWorkspaceRequest) (*insightifyv1.GetUiWorkspaceResponse, error) {
	if s == nil || s.workspaces == nil {
		return nil, fmt.Errorf("ui workspace service is not available")
	}
	projectID := strings.TrimSpace(req.GetProjectId())
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	view, err := s.workspaces.Ensure(projectID)
	if err != nil {
		return nil, err
	}
	return &insightifyv1.GetUiWorkspaceResponse{
		Workspace: toProtoWorkspace(view.Workspace),
		Tabs:      toProtoTabs(view.Tabs),
	}, nil
}

func (s *Service) ListTabs(_ context.Context, req *insightifyv1.ListUiTabsRequest) (*insightifyv1.ListUiTabsResponse, error) {
	if s == nil || s.workspaces == nil {
		return nil, fmt.Errorf("ui workspace service is not available")
	}
	projectID := strings.TrimSpace(req.GetProjectId())
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	view, err := s.workspaces.Ensure(projectID)
	if err != nil {
		return nil, err
	}
	return &insightifyv1.ListUiTabsResponse{
		Workspace: toProtoWorkspace(view.Workspace),
		Tabs:      toProtoTabs(view.Tabs),
	}, nil
}

func (s *Service) CreateTab(_ context.Context, req *insightifyv1.CreateUiTabRequest) (*insightifyv1.CreateUiTabResponse, error) {
	if s == nil || s.workspaces == nil {
		return nil, fmt.Errorf("ui workspace service is not available")
	}
	projectID := strings.TrimSpace(req.GetProjectId())
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	view, tab, err := s.workspaces.CreateTab(projectID, strings.TrimSpace(req.GetTitle()))
	if err != nil {
		return nil, err
	}
	return &insightifyv1.CreateUiTabResponse{
		Workspace: toProtoWorkspace(view.Workspace),
		Tab:       toProtoTab(tab),
		Tabs:      toProtoTabs(view.Tabs),
	}, nil
}

func (s *Service) SelectTab(_ context.Context, req *insightifyv1.SelectUiTabRequest) (*insightifyv1.SelectUiTabResponse, error) {
	if s == nil || s.workspaces == nil {
		return nil, fmt.Errorf("ui workspace service is not available")
	}
	projectID := strings.TrimSpace(req.GetProjectId())
	tabID := strings.TrimSpace(req.GetTabId())
	if projectID == "" || tabID == "" {
		return nil, fmt.Errorf("project_id and tab_id are required")
	}
	view, err := s.workspaces.SelectTab(projectID, tabID)
	if err != nil {
		return nil, err
	}
	return &insightifyv1.SelectUiTabResponse{
		Workspace: toProtoWorkspace(view.Workspace),
		Tabs:      toProtoTabs(view.Tabs),
	}, nil
}

func (s *Service) Set(runID string, node *insightifyv1.UiNode) {
	if s == nil || s.store == nil || strings.TrimSpace(runID) == "" || node == nil {
		return
	}
	_, _, _ = s.store.ApplyOps(context.Background(), runID, 0, []*insightifyv1.UiOp{
		{
			Action: &insightifyv1.UiOp_UpsertNode{
				UpsertNode: &insightifyv1.UiUpsertNode{Node: node},
			},
		},
	})
}

func (s *Service) Get(runID string) *insightifyv1.UiNode {
	if s == nil || s.store == nil || strings.TrimSpace(runID) == "" {
		return nil
	}
	doc, err := s.store.GetDocument(context.Background(), runID)
	if err != nil {
		return nil
	}
	if doc == nil || len(doc.GetNodes()) == 0 {
		return nil
	}
	return doc.GetNodes()[0]
}

func (s *Service) Clear(runID string) {
	if s == nil || s.store == nil || strings.TrimSpace(runID) == "" {
		return
	}
	_, _, _ = s.store.ApplyOps(context.Background(), runID, 0, []*insightifyv1.UiOp{
		{
			Action: &insightifyv1.UiOp_ClearNodes{
				ClearNodes: &insightifyv1.UiClearNodes{},
			},
		},
	})
}

func toProtoWorkspace(ws uiworkspacerepo.Workspace) *insightifyv1.UiWorkspace {
	return &insightifyv1.UiWorkspace{
		WorkspaceId: strings.TrimSpace(ws.WorkspaceID),
		ProjectId:   strings.TrimSpace(ws.ProjectID),
		Name:        strings.TrimSpace(ws.Name),
		ActiveTabId: strings.TrimSpace(ws.ActiveTabID),
	}
}

func toProtoTab(t uiworkspacerepo.Tab) *insightifyv1.UiWorkspaceTab {
	return &insightifyv1.UiWorkspaceTab{
		TabId:           strings.TrimSpace(t.TabID),
		WorkspaceId:     strings.TrimSpace(t.WorkspaceID),
		Title:           strings.TrimSpace(t.Title),
		RunId:           strings.TrimSpace(t.RunID),
		OrderIndex:      t.OrderIndex,
		IsPinned:        t.IsPinned,
		CreatedAtUnixMs: t.CreatedAtUnixMs,
	}
}

func toProtoTabs(tabs []uiworkspacerepo.Tab) []*insightifyv1.UiWorkspaceTab {
	if len(tabs) == 0 {
		return nil
	}
	out := make([]*insightifyv1.UiWorkspaceTab, 0, len(tabs))
	for _, t := range tabs {
		out = append(out, toProtoTab(t))
	}
	return out
}
