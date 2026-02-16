package projectstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"insightify/internal/gateway/ent"

	"github.com/hashicorp/golang-lru/v2"
)

type State struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	UserID      string `json:"user_id"`
	Repo        string `json:"repo"`
	IsActive    bool   `json:"is_active"`
}

type ProjectArtifact struct {
	ID        int       `json:"id"`
	ProjectID string    `json:"project_id"`
	RunID     string    `json:"run_id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	path string
	db   *sql.DB // Legacy field, kept if needed but we prefer ent
	ent  *ent.Client

	loadOnce sync.Once
	mu       sync.RWMutex
	byID     map[string]State
	
	// Metadata Cache (Read-Through)
	artifactCache *lru.Cache[string, []ProjectArtifact]
}

func NewFromEnv(path string) *Store {
	return &Store{
		path: path,
		byID: make(map[string]State),
	}
}

func NewPostgresStore(client *ent.Client, cache *lru.Cache[string, []ProjectArtifact]) (*Store, error) {
	return &Store{
		ent:           client,
		artifactCache: cache,
	}, nil
}

func (s *Store) EnsureLoaded() {
	if s == nil {
		return
	}
	if s.ent != nil {
		_ = s.ensureEntSchema()
		return
	}
	s.ensureLoadedFile()
}

// File backed implementation helpers
func (s *Store) ensureLoadedFile() {
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

func (s *Store) saveFile() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var states []State
	for _, state := range s.byID {
		states = append(states, state)
	}

	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// Public Methods

func (s *Store) Save() error {
	if s == nil {
		return nil
	}
	if s.ent != nil {
		// Ent operations are immediate/transactional, no manual save needed.
		return nil
	}
	return s.saveFile()
}

func (s *Store) Get(projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.getEnt(projectID)
	}
	s.ensureLoadedFile()
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.byID[projectID]
	return state, ok
}

func (s *Store) Put(state State) {
	if s == nil {
		return
	}
	if s.ent != nil {
		s.putEnt(state)
		return
	}
	s.ensureLoadedFile()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[state.ProjectID] = state
	_ = s.saveFile()
}

func (s *Store) Update(projectID string, update func(*State)) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.updateEnt(projectID, update)
	}
	s.ensureLoadedFile()
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.byID[projectID]
	if !ok {
		return State{}, false
	}

	update(&state)
	s.byID[projectID] = state
	_ = s.saveFile()
	return state, true
}

func (s *Store) ListByUser(userID string) []State {
	if s == nil {
		return nil
	}
	if s.ent != nil {
		return s.listByUserEnt(userID)
	}
	s.ensureLoadedFile()
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []State
	for _, state := range s.byID {
		if state.UserID == userID {
			out = append(out, state)
		}
	}
	return out
}

func (s *Store) GetActiveByUser(userID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.getActiveByUserEnt(userID)
	}
	s.ensureLoadedFile()
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, state := range s.byID {
		if state.UserID == userID && state.IsActive {
			return state, true
		}
	}
	return State{}, false
}

func (s *Store) SetActiveForUser(userID, projectID string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	if s.ent != nil {
		return s.setActiveForUserEnt(userID, projectID)
	}
	s.ensureLoadedFile()
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deactivate others
	for id, state := range s.byID {
		if state.UserID == userID && state.IsActive {
			state.IsActive = false
			s.byID[id] = state
		}
	}

	// Activate target
	state, ok := s.byID[projectID]
	if !ok {
		_ = s.saveFile() // Save the deactivation changes even if target not found?
		return State{}, false
	}

	state.IsActive = true
	s.byID[projectID] = state
	_ = s.saveFile()

	return state, true
}

// Artifact Metadata Methods

func (s *Store) AddArtifact(artifact ProjectArtifact) error {
	if s == nil {
		return nil
	}
	if s.ent != nil {
		err := s.addArtifactEnt(artifact)
		if err == nil && s.artifactCache != nil {
			s.artifactCache.Remove(artifact.ProjectID)
		}
		return err
	}
	// File backend artifact support is limited/not implemented in original file
	return nil
}

func (s *Store) ListArtifacts(projectID string) ([]ProjectArtifact, error) {
	if s == nil {
		return nil, nil
	}
	
	// Check cache first (Read-Through)
	if s.artifactCache != nil {
		if cached, ok := s.artifactCache.Get(projectID); ok {
			return cached, nil
		}
	}

	if s.ent != nil {
		artifacts, err := s.listArtifactsEnt(projectID)
		if err != nil {
			return nil, err
		}
		if s.artifactCache != nil {
			s.artifactCache.Add(projectID, artifacts)
		}
		return artifacts, nil
	}
	return nil, nil
}
