package ui

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	uirepo "insightify/internal/gateway/repository/ui"
)

// Service provides UI node state operations for gateway services.
type Service struct {
	store *uirepo.Store
}

func New(store *uirepo.Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetDocument(_ context.Context, req *insightifyv1.GetUiDocumentRequest) (*insightifyv1.GetUiDocumentResponse, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("ui service is not available")
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	doc := s.store.GetDocument(runID)
	if doc == nil {
		doc = &insightifyv1.UiDocument{RunId: runID}
	}
	return &insightifyv1.GetUiDocumentResponse{Document: doc}, nil
}

func (s *Service) ApplyOps(_ context.Context, req *insightifyv1.ApplyUiOpsRequest) (*insightifyv1.ApplyUiOpsResponse, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("ui service is not available")
	}
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	doc, conflict, err := s.store.ApplyOps(runID, req.GetBaseVersion(), req.GetOps())
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

// Compatibility helpers used by worker service.
func (s *Service) Set(runID string, node *insightifyv1.UiNode) {
	if s == nil || s.store == nil || strings.TrimSpace(runID) == "" || node == nil {
		return
	}
	_, _, _ = s.store.ApplyOps(runID, 0, []*insightifyv1.UiOp{
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
	doc := s.store.GetDocument(runID)
	if doc == nil || len(doc.GetNodes()) == 0 {
		return nil
	}
	return doc.GetNodes()[0]
}

func (s *Service) Clear(runID string) {
	if s == nil || s.store == nil || strings.TrimSpace(runID) == "" {
		return
	}
	_, _, _ = s.store.ApplyOps(runID, 0, []*insightifyv1.UiOp{
		{
			Action: &insightifyv1.UiOp_ClearNodes{
				ClearNodes: &insightifyv1.UiClearNodes{},
			},
		},
	})
}
