package userinteraction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type Service struct {
	mu    sync.Mutex
	state map[string]*sessionState
}

type sessionState struct {
	interactionID string
	closed        bool
	updatedAt     time.Time
}

func New() *Service {
	return &Service{
		state: make(map[string]*sessionState),
	}
}

func (s *Service) Wait(_ context.Context, req *insightifyv1.WaitRequest) (*insightifyv1.WaitResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreateLocked(runID)
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.updatedAt = time.Now()

	return &insightifyv1.WaitResponse{
		Waiting:       !st.closed,
		InteractionId: st.interactionID,
		Closed:        st.closed,
	}, nil
}

func (s *Service) Send(_ context.Context, req *insightifyv1.SendRequest) (*insightifyv1.SendResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	input := strings.TrimSpace(req.GetInput())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if input == "" {
		return nil, fmt.Errorf("input is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreateLocked(runID)
	if st.closed {
		return &insightifyv1.SendResponse{
			Accepted:      false,
			InteractionId: st.interactionID,
		}, nil
	}
	if interactionID := strings.TrimSpace(req.GetInteractionId()); interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.updatedAt = time.Now()

	return &insightifyv1.SendResponse{
		Accepted:      true,
		InteractionId: st.interactionID,
	}, nil
}

func (s *Service) Close(_ context.Context, req *insightifyv1.CloseRequest) (*insightifyv1.CloseResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreateLocked(runID)
	if interactionID := strings.TrimSpace(req.GetInteractionId()); interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.closed = true
	st.updatedAt = time.Now()

	return &insightifyv1.CloseResponse{
		Closed: true,
	}, nil
}

func (s *Service) getOrCreateLocked(runID string) *sessionState {
	key := runID
	if st, ok := s.state[key]; ok {
		return st
	}
	st := &sessionState{}
	s.state[key] = st
	return st
}

func newInteractionID() string {
	return fmt.Sprintf("interaction-%d", time.Now().UnixNano())
}
