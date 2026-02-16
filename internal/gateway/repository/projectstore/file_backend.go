package projectstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func (s *Store) addArtifactFile(artifact ProjectArtifact) error {
	s.ensureLoadedFile()
	if artifact.ProjectID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.saveFile() // saveFile unlocks
	// saveFile calls ensureLoadedFile -> RLock/RUnlock
	// Wait, saveFile calls mu.RLock() so we should be careful about locking.
	// saveFile implementation:
	// func (s *Store) saveFile() {
	// 	s.ensureLoadedFile()
	// 	s.mu.RLock() ... s.mu.RUnlock() ...
	// }
	// So we should NOT hold Lock when calling saveFile if saveFile takes RLock.
	// Actually saveFile logic in file_backend.go (lines 32-47) takes RLock.
	// We hold Lock here. RLock while holding Lock is generally okay in RWMutex if it's the same goroutine? No, RWMutex doesn't allow recursive read locks if write lock is held.
	// So we must Unlock before calling saveFile.
	
	state, ok := s.byID[artifact.ProjectID]
	if !ok {
		// If project doesn't exist, maybe we shouldn't add artifact? 
		// Or creating a stub state? For now let's assume project exists.
		s.mu.Unlock()
		return nil
	}
	
	// Check for duplicates
	for _, a := range state.Artifacts {
		if a.RunID == artifact.RunID && a.Path == artifact.Path {
			s.mu.Unlock()
			return nil
		}
	}
	
	// Add artifact
	artifact.CreatedAt = time.Now()
	// ID generation simplified
	artifact.ID = len(state.Artifacts) + 1 
	state.Artifacts = append(state.Artifacts, artifact)
	s.byID[artifact.ProjectID] = state
	
	s.mu.Unlock()
	s.saveFile()
	return nil
}

func (s *Store) listArtifactsFile(projectID string) ([]ProjectArtifact, error) {
	s.ensureLoadedFile()
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.byID[pid]
	if !ok {
		return nil, nil
	}
	
	// Sort by CreatedAt DESC
	// coping to avoid mutation issues if we sort in place (though slice is copy, elements are shared, but here struct is by value)
	out := make([]ProjectArtifact, len(state.Artifacts))
	copy(out, state.Artifacts)
	
	// Simple bubble sort or just return as is? 
	// The requirement says "ORDER BY created_at DESC".
	// Since we append, it is ASC. We can verify or just reverse.
	// Let's do a reverse iterate to return.
	
	reversed := make([]ProjectArtifact, 0, len(out))
	for i := len(out) - 1; i >= 0; i-- {
		reversed = append(reversed, out[i])
	}
	return reversed, nil
}
