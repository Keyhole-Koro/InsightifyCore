package project

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"insightify/internal/gateway/entity"
)

type FileStore struct {
	path string

	loadOnce sync.Once
	mu       sync.RWMutex
	byID     map[string]State
}

func NewFromEnv(path string) *FileStore {
	return &FileStore{
		path: path,
		byID: make(map[string]State),
	}
}

func (s *FileStore) EnsureLoaded() {
	if s == nil {
		return
	}
	s.ensureLoaded()
}

func (s *FileStore) Save() error {
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

func (s *FileStore) Get(projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.byID[projectID]
	return state, ok
}

func (s *FileStore) Put(state State) {
	if s == nil {
		return
	}
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[state.ProjectID] = state
	_ = s.saveLocked()
}

func (s *FileStore) Update(projectID string, update func(*State)) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.byID[projectID]
	if !ok {
		return State{}, false
	}

	update(&state)
	s.byID[projectID] = state
	_ = s.saveLocked()
	return state, true
}

func (s *FileStore) ListByUser(userID entity.UserID) []State {
	if s == nil {
		return nil
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
	return out
}

func (s *FileStore) GetActiveByUser(userID entity.UserID) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.ensureLoaded()
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, state := range s.byID {
		if state.UserID.String() == userID.String() && state.IsActive {
			return state, true
		}
	}
	return State{}, false
}

func (s *FileStore) SetActiveForUser(userID entity.UserID, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
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
		_ = s.saveLocked()
		return State{}, false
	}

	state.IsActive = true
	s.byID[projectID] = state
	_ = s.saveLocked()
	return state, true
}

func (s *FileStore) AddArtifact(_ ProjectArtifact) error {
	// File backend artifact support is currently not implemented.
	return nil
}

func (s *FileStore) ListArtifacts(_ string) ([]ProjectArtifact, error) {
	return nil, nil
}

func (s *FileStore) ensureLoaded() {
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

func (s *FileStore) saveLocked() error {
	states := make([]State, 0, len(s.byID))
	for _, state := range s.byID {
		states = append(states, state)
	}
	return s.writeStates(states)
}

func (s *FileStore) writeStates(states []State) error {
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}
