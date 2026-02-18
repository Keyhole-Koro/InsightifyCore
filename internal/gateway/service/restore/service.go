package restore

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	uirepo "insightify/internal/gateway/repository/ui"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
)

const (
	ReasonResolved = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_RESOLVED
	ReasonNoTab    = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_NO_TAB
	ReasonNoRun    = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_NO_RUN
	ReasonError    = insightifyv1.UiRestoreReason_UI_RESTORE_REASON_ERROR
)

type Result struct {
	ProjectID string
	TabID     string
	RunID     string
	Found     bool
	Restored  bool
	Reason    insightifyv1.UiRestoreReason
	Document  *insightifyv1.UiDocument
}

type Service struct {
	uiStore    uirepo.Store
	workspaces *gatewayuiworkspace.Service
}

func New(uiStore uirepo.Store, workspaces *gatewayuiworkspace.Service) *Service {
	return &Service{
		uiStore:    uiStore,
		workspaces: workspaces,
	}
}

func (s *Service) ResolveProjectTabDocument(ctx context.Context, projectID, preferredTabID string) (Result, error) {
	if s == nil || s.uiStore == nil {
		return Result{}, fmt.Errorf("restore service ui store is not available")
	}
	if s.workspaces == nil {
		return Result{}, fmt.Errorf("restore service workspace is not available")
	}
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return Result{}, fmt.Errorf("project_id is required")
	}

	tabPref := strings.TrimSpace(preferredTabID)
	if tabPref == "" {
		tabPref = gatewayuiworkspace.DefaultTabID
	}

	_, tab, ok, err := s.workspaces.ResolveTab(pid, tabPref)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{
			ProjectID: pid,
			Found:     false,
			Restored:  false,
			Reason:    ReasonNoTab,
		}, nil
	}

	tabID := strings.TrimSpace(tab.TabID)
	runID := strings.TrimSpace(tab.RunID)
	if runID == "" {
		return Result{
			ProjectID: pid,
			TabID:     tabID,
			Found:     false,
			Restored:  false,
			Reason:    ReasonNoRun,
		}, nil
	}

	doc, err := s.uiStore.GetDocument(ctx, runID)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ProjectID: pid,
		TabID:     tabID,
		RunID:     runID,
		Found:     true,
		Restored:  true,
		Reason:    ReasonResolved,
		Document:  doc,
	}, nil
}

func (r Result) ToRestoreProtoResponse() *insightifyv1.RestoreUiResponse {
	return &insightifyv1.RestoreUiResponse{
		Found:     r.Found,
		Restored:  r.Restored,
		Reason:    r.Reason,
		ProjectId: strings.TrimSpace(r.ProjectID),
		TabId:     strings.TrimSpace(r.TabID),
		RunId:     strings.TrimSpace(r.RunID),
		Document:  r.Document,
	}
}
