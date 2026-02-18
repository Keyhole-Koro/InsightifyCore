package project

import (
	"context"
	"fmt"
	"sync"

	"insightify/internal/gateway/entity"
)

// MemoryStore is an in-memory origin/fallback for project repository contracts.
type MemoryStore struct {
	mu   sync.RWMutex
	byID map[string]State
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID: make(map[string]State),
	}
}

func (s *MemoryStore) EnsureLoaded(_ context.Context) {}

func (s *MemoryStore) Save(_ context.Context) error { return nil }

func (s *MemoryStore) Get(_ context.Context, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.byID[projectID]
	return st, ok
}

func (s *MemoryStore) Put(_ context.Context, state State) error {
	if s == nil {
		return fmt.Errorf("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[state.ProjectID] = state
	return nil
}

func (s *MemoryStore) Update(_ context.Context, projectID string, update func(*State)) (State, bool, error) {
	if s == nil {
		return State{}, false, fmt.Errorf("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.byID[projectID]
	if !ok {
		return State{}, false, nil
	}
	update(&st)
	s.byID[projectID] = st
	return st, true, nil
}

func (s *MemoryStore) ListByUser(_ context.Context, userID entity.UserID) ([]State, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]State, 0, 8)
	for _, st := range s.byID {
		if st.UserID.String() == userID.String() {
			out = append(out, st)
		}
	}
	return out, nil
}

func (s *MemoryStore) GetActiveByUser(_ context.Context, userID entity.UserID) (State, bool, error) {
	if s == nil {
		return State{}, false, fmt.Errorf("store is nil")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, st := range s.byID {
		if st.UserID.String() == userID.String() && st.IsActive {
			return st, true, nil
		}
	}
	return State{}, false, nil
}

func (s *MemoryStore) SetActiveForUser(_ context.Context, userID entity.UserID, projectID string) (State, bool, error) {
	if s == nil {
		return State{}, false, fmt.Errorf("store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, st := range s.byID {
		if st.UserID.String() == userID.String() && st.IsActive {
			st.IsActive = false
			s.byID[id] = st
		}
	}
	st, ok := s.byID[projectID]
	if !ok {
		return State{}, false, nil
	}
	st.IsActive = true
	s.byID[projectID] = st
	return st, true, nil
}

func (s *MemoryStore) AddArtifact(_ context.Context, _ ProjectArtifact) error { return nil }

func (s *MemoryStore) ListArtifacts(_ context.Context, _ string) ([]ProjectArtifact, error) {
	return nil, nil
}
