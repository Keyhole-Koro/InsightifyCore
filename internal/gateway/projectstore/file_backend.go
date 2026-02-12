package projectstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) ensureLoadedFile() {
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

func (s *Store) saveFile() {
	s.ensureLoadedFile()
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

func (s *Store) getFile(projectID string) (State, bool) {
	s.ensureLoadedFile()
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

func (s *Store) putFile(state State) {
	s.ensureLoadedFile()
	normalized := normalizeState(state)
	if normalized.ProjectID == "" {
		return
	}
	s.mu.Lock()
	s.byID[normalized.ProjectID] = normalized
	s.mu.Unlock()
}

func (s *Store) updateFile(projectID string, update func(*State)) (State, bool) {
	s.ensureLoadedFile()
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

func (s *Store) listByUserFile(userID string) []State {
	s.ensureLoadedFile()
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

func (s *Store) getActiveByUserFile(userID string) (State, bool) {
	s.ensureLoadedFile()
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

func (s *Store) setActiveForUserFile(userID, projectID string) (State, bool) {
	s.ensureLoadedFile()
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
