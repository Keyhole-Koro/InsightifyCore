package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"insightify/internal/gateway/entity"
)

// DiskStore persists project state as JSON on local disk.
type DiskStore struct {
	path string

	loadOnce sync.Once
	mu       sync.RWMutex
	byID     map[string]State
}

func NewDiskStore(path string) *DiskStore {
	return &DiskStore{
		path: path,
		byID: make(map[string]State),
	}
}

func (s *DiskStore) EnsureLoaded(_ context.Context) {
	if s == nil {
		return
	}
	s.ensureLoaded()
}

func (s *DiskStore) Save(_ context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	states := make([]State, 0, len(s.byID))
	for _, state := range s.byID {
		states = append(states, state)
	}
	s.mu.RUnlock()
	return s.writeStates(states)
}

func (s *DiskStore) Get(_ context.Context, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.byID[projectID]
	return state, ok
}

func (s *DiskStore) Put(_ context.Context, state State) error {
	if s == nil {
		return nil
	}
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[state.ProjectID] = state
	return s.saveLocked()
}

func (s *DiskStore) Update(_ context.Context, projectID string, update func(*State)) (State, bool, error) {
	if s == nil {
		return State{}, false, nil
	}
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.byID[projectID]
	if !ok {
		return State{}, false, nil
	}
	update(&state)
	s.byID[projectID] = state
	return state, true, s.saveLocked()
}

func (s *DiskStore) ListByUser(_ context.Context, userID entity.UserID) ([]State, error) {
	if s == nil {
		return nil, nil
	}
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]State, 0)
	for _, state := range s.byID {
		if state.UserID.String() == userID.String() {
			out = append(out, state)
		}
	}
	return out, nil
}

func (s *DiskStore) GetActiveByUser(_ context.Context, userID entity.UserID) (State, bool, error) {
	if s == nil {
		return State{}, false, nil
	}
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, state := range s.byID {
		if state.UserID.String() == userID.String() && state.IsActive {
			return state, true, nil
		}
	}
	return State{}, false, nil
}

func (s *DiskStore) SetActiveForUser(_ context.Context, userID entity.UserID, projectID string) (State, bool, error) {
	if s == nil {
		return State{}, false, nil
	}
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, state := range s.byID {
		if state.UserID.String() == userID.String() && state.IsActive {
			state.IsActive = false
			s.byID[id] = state
		}
	}

	state, ok := s.byID[projectID]
	if !ok {
		return State{}, false, s.saveLocked()
	}
	state.IsActive = true
	s.byID[projectID] = state
	return state, true, s.saveLocked()
}

func (s *DiskStore) AddArtifact(_ context.Context, _ ProjectArtifact) error { return nil }

func (s *DiskStore) ListArtifacts(_ context.Context, _ string) ([]ProjectArtifact, error) {
	return nil, nil
}

func (s *DiskStore) ensureLoaded() {
	s.loadOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		data, err := os.ReadFile(s.path)
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			fmt.Printf("failed to read project store: %v\n", err)
			return
		}

		var states []State
		if err := json.Unmarshal(data, &states); err != nil {
			fmt.Printf("failed to unmarshal project store: %v\n", err)
			return
		}
		for _, state := range states {
			s.byID[state.ProjectID] = state
		}
	})
}

func (s *DiskStore) saveLocked() error {
	states := make([]State, 0, len(s.byID))
	for _, state := range s.byID {
		states = append(states, state)
	}
	return s.writeStates(states)
}

func (s *DiskStore) writeStates(states []State) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
