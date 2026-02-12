package projectstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type State struct {
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	UserID      string `json:"user_id"`
	Repo        string `json:"repo"`
	IsActive    bool   `json:"is_active,omitempty"`
	Running     bool   `json:"running,omitempty"`
	ActiveRunID string `json:"active_run_id,omitempty"`
}

type Store struct {
	path string

	loadOnce sync.Once
	mu       sync.RWMutex
	byID     map[string]State
}

func New(path string) *Store {
	return &Store{
		path: path,
		byID: make(map[string]State),
	}
}

func (s *Store) EnsureLoaded() {
	if s == nil {
		return
	}
	s.loadOnce.Do(func() {
		b, err := os.ReadFile(s.path)
		if err != nil {
			return
		}
		var rows []State
		if err := json.Unmarshal(b, &rows); err != nil {
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, row := range rows {
			id := strings.TrimSpace(row.ProjectID)
			if id == "" {
				continue
			}
			s.byID[id] = normalizeState(row)
		}
	})
}

func (s *Store) Save() {
	if s == nil {
		return
	}
	s.EnsureLoaded()
	s.mu.RLock()
	rows := make([]State, 0, len(s.byID))
	for _, state := range s.byID {
		rows = append(rows, normalizeState(state))
	}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	_ = os.WriteFile(s.path, b, 0o644)
}

func (s *Store) Get(projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.EnsureLoaded()
	id := strings.TrimSpace(projectID)
	if id == "" {
		return State{}, false
	}
	s.mu.RLock()
	state, ok := s.byID[id]
	s.mu.RUnlock()
	if !ok {
		return State{}, false
	}
	return normalizeState(state), true
}

func (s *Store) Put(state State) {
	if s == nil {
		return
	}
	s.EnsureLoaded()
	normalized := normalizeState(state)
	if normalized.ProjectID == "" {
		return
	}
	s.mu.Lock()
	s.byID[normalized.ProjectID] = normalized
	s.mu.Unlock()
}

func (s *Store) Update(projectID string, update func(*State)) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.EnsureLoaded()
	id := strings.TrimSpace(projectID)
	if id == "" {
		return State{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.byID[id]
	if !ok {
		return State{}, false
	}
	update(&state)
	state.ProjectID = id
	state = normalizeState(state)
	s.byID[id] = state
	return state, true
}

func (s *Store) ListByUser(userID string) []State {
	if s == nil {
		return nil
	}
	s.EnsureLoaded()
	uid := strings.TrimSpace(userID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]State, 0, len(s.byID))
	for _, state := range s.byID {
		if uid != "" && strings.TrimSpace(state.UserID) != uid {
			continue
		}
		out = append(out, normalizeState(state))
	}
	return out
}

func (s *Store) GetActiveByUser(userID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.EnsureLoaded()
	uid := strings.TrimSpace(userID)
	if uid == "" {
		return State{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, state := range s.byID {
		if strings.TrimSpace(state.UserID) != uid {
			continue
		}
		if state.IsActive {
			return normalizeState(state), true
		}
	}
	return State{}, false
}

func (s *Store) SetActiveForUser(userID, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.EnsureLoaded()
	uid := strings.TrimSpace(userID)
	pid := strings.TrimSpace(projectID)
	if uid == "" || pid == "" {
		return State{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected State
	var found bool
	for key, state := range s.byID {
		if strings.TrimSpace(state.UserID) != uid {
			continue
		}
		if strings.TrimSpace(state.ProjectID) == pid || key == pid {
			state.IsActive = true
			selected = state
			found = true
		} else {
			state.IsActive = false
		}
		s.byID[key] = normalizeState(state)
	}
	return normalizeState(selected), found
}

func normalizeState(state State) State {
	state.ProjectID = strings.TrimSpace(state.ProjectID)
	state.ProjectName = strings.TrimSpace(state.ProjectName)
	state.UserID = strings.TrimSpace(state.UserID)
	state.Repo = strings.TrimSpace(state.Repo)
	state.ActiveRunID = strings.TrimSpace(state.ActiveRunID)
	if state.ProjectName == "" {
		state.ProjectName = "Project"
	}
	if state.ActiveRunID == "" {
		state.Running = false
	}
	return state
}
