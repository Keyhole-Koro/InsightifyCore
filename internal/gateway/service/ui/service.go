package ui

import (
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

func (s *Service) Set(runID string, node *insightifyv1.UiNode) {
	s.store.Set(runID, node)
}

func (s *Service) Get(runID string) *insightifyv1.UiNode {
	return s.store.Get(runID)
}

func (s *Service) Clear(runID string) {
	s.store.Clear(runID)
}
