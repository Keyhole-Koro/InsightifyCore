package project

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"insightify/internal/gateway/repository/projectstore"
	runtimepkg "insightify/internal/workerruntime"
)

// Service implements Project business logic and owns all project state.
type Service struct {
	store *projectstore.Store

	runCtxMu sync.RWMutex
	runCtx   map[string]*runtimepkg.ProjectRuntime
}

// New creates a project service backed by the given store.
func New(store *projectstore.Store) *Service {
	return &Service{
		store:  store,
		runCtx: make(map[string]*runtimepkg.ProjectRuntime),
	}
}

// Store returns the underlying project persistence store.
func (s *Service) Store() *projectstore.Store { return s.store }

// ---------------------------------------------------------------------------
// Business Logic
// ---------------------------------------------------------------------------

// Entry is the public type for project entry (was unexported 'entry').
type Entry struct {
	State  projectstore.State
	RunCtx *runtimepkg.ProjectRuntime
}

func (s *Service) ListProjects(_ context.Context, userID string) ([]Entry, string, error) {
	s.store.EnsureLoaded()

	projects := s.listByUser(userID)
	// Sort by ProjectID
	sort.Slice(projects, func(i, j int) bool {
		return strings.TrimSpace(projects[i].State.ProjectID) < strings.TrimSpace(projects[j].State.ProjectID)
	})

	var activeID string
	for _, p := range projects {
		if p.State.IsActive && activeID == "" {
			activeID = p.State.ProjectID
		}
	}
	return projects, activeID, nil
}

func (s *Service) CreateProject(_ context.Context, userID, projectName string) (Entry, error) {
	s.store.EnsureLoaded()

	if projectName == "" {
		projectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}

	projectID := fmt.Sprintf("project-%d", time.Now().UnixNano())

	var runCtx *runtimepkg.ProjectRuntime
	ctx, err := runtimepkg.NewProjectRuntime("", projectID)
	if err != nil {
		return Entry{}, fmt.Errorf("failed to create run context: %w", err)
	}
	runCtx = ctx

	p := Entry{
		State: projectstore.State{
			ProjectID:   projectID,
			ProjectName: projectName,
			UserID:      userID,
			Repo:        "",
			IsActive:    true,
		},
		RunCtx: runCtx,
	}
	s.put(p)
	s.setActiveForUser(userID, projectID)
	s.store.Save()

	got, _ := s.get(projectID)
	return got, nil
}

func (s *Service) SelectProject(_ context.Context, userID, projectID string) (Entry, error) {
	s.store.EnsureLoaded()

	p, ok := s.get(projectID)
	if !ok {
		return Entry{}, fmt.Errorf("project %s not found", projectID)
	}
	if strings.TrimSpace(p.State.UserID) != userID {
		return Entry{}, fmt.Errorf("project %s does not belong to user %s", projectID, userID)
	}

	selected, ok := s.setActiveForUser(userID, projectID)
	if !ok {
		return Entry{}, fmt.Errorf("project %s not found", projectID)
	}
	s.store.Save()
	return selected, nil
}

func (s *Service) EnsureProject(_ context.Context, userID, projectID string) (Entry, error) {
	s.store.EnsureLoaded()

	if userID == "" {
		userID = "demo-user"
	}

	var (
		p       Entry
		existed bool
	)

	// Resolve project.
	if projectID == "" {
		if active, ok := s.getActiveByUser(userID); ok {
			projectID = active.State.ProjectID
		}
	}
	if projectID != "" {
		p, existed = s.get(projectID)
	}
	if !existed {
		if projectID == "" {
			projectID = fmt.Sprintf("project-%d", time.Now().UnixNano())
		}
		p = Entry{
			State: projectstore.State{
				ProjectID:   projectID,
				ProjectName: fmt.Sprintf("Project %d", time.Now().Unix()%100000),
				UserID:      userID,
				IsActive:    true,
			},
		}
	}

	p.State.UserID = userID
	p.State.ProjectID = projectID
	if strings.TrimSpace(p.State.ProjectName) == "" {
		p.State.ProjectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}

	// Ensure run context.
	if !s.hasRequiredWorkers(p.RunCtx) {
		ctx, err := runtimepkg.NewProjectRuntime(p.State.Repo, projectID)
		if err != nil {
			return Entry{}, fmt.Errorf("failed to create run context: %w", err)
		}
		p.RunCtx = ctx
	}

	s.put(p)
	s.setActiveForUser(p.State.UserID, p.State.ProjectID)
	s.store.Save()

	got, _ := s.get(projectID)
	return got, nil
}

// ---------------------------------------------------------------------------
// State management (absorbed from runtime.App)
// ---------------------------------------------------------------------------

func (s *Service) get(projectID string) (Entry, bool) {
	state, ok := s.store.Get(projectID)
	if !ok {
		return Entry{}, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[strings.TrimSpace(projectID)]
	s.runCtxMu.RUnlock()
	return Entry{State: state, RunCtx: ctx}, true
}

func (s *Service) put(e Entry) {
	if strings.TrimSpace(e.State.ProjectID) == "" {
		return
	}
	s.store.Put(e.State)
	s.runCtxMu.Lock()
	s.runCtx[e.State.ProjectID] = e.RunCtx
	s.runCtxMu.Unlock()
}

func (s *Service) listByUser(userID string) []Entry {
	states := s.store.ListByUser(userID)
	out := make([]Entry, 0, len(states))
	s.runCtxMu.RLock()
	for _, st := range states {
		if !isProjectID(st.ProjectID) {
			continue
		}
		out = append(out, Entry{State: st, RunCtx: s.runCtx[st.ProjectID]})
	}
	s.runCtxMu.RUnlock()
	return out
}

func (s *Service) getActiveByUser(userID string) (Entry, bool) {
	st, ok := s.store.GetActiveByUser(userID)
	if !ok {
		return Entry{}, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[st.ProjectID]
	s.runCtxMu.RUnlock()
	return Entry{State: st, RunCtx: ctx}, true
}

func (s *Service) setActiveForUser(userID, projectID string) (Entry, bool) {
	st, ok := s.store.SetActiveForUser(userID, projectID)
	if !ok {
		return Entry{}, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[st.ProjectID]
	s.runCtxMu.RUnlock()
	return Entry{State: st, RunCtx: ctx}, true
}

// ---------------------------------------------------------------------------
// Public accessors (used by run.Service via ProjectReader interface)
// ---------------------------------------------------------------------------

// GetRunContext returns the run context for a project.
func (s *Service) GetRunContext(projectID string) (*runtimepkg.ProjectRuntime, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[projectID]
	s.runCtxMu.RUnlock()
	return ctx, ctx != nil
}

// GetEntry returns the project state as a facade for other packages.
func (s *Service) GetEntry(projectID string) (State, bool) {
	e, ok := s.get(projectID)
	if !ok {
		return State{}, false
	}
	return State{
		ProjectID:   e.State.ProjectID,
		ProjectName: e.State.ProjectName,
		UserID:      e.State.UserID,
		Repo:        e.State.Repo,
		IsActive:    e.State.IsActive,
		RunCtx:      e.RunCtx,
	}, true
}

// EnsureRunContext ensures a project has a valid run context with required workers.
func (s *Service) EnsureRunContext(projectID string) (*runtimepkg.ProjectRuntime, error) {
	e, ok := s.get(projectID)
	if !ok {
		return nil, fmt.Errorf("project %s not found", projectID)
	}
	if e.RunCtx != nil && s.hasRequiredWorkers(e.RunCtx) {
		return e.RunCtx, nil
	}
	ctx, err := runtimepkg.NewProjectRuntime(e.State.Repo, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to restore run context: %w", err)
	}
	e.RunCtx = ctx
	s.put(e)
	return ctx, nil
}

func (s *Service) hasRequiredWorkers(env *runtimepkg.ProjectRuntime) bool {
	if env == nil || env.Resolver == nil {
		return false
	}
	_, hasBootstrap := env.Resolver.Get("bootstrap")
	_, hasTestLLM := env.Resolver.Get("testllmChatNode")
	return hasBootstrap && hasTestLLM
}

func isProjectID(id string) bool {
	return strings.HasPrefix(strings.TrimSpace(id), "project-")
}

// State is a public read-only view of project state.
type State struct {
	ProjectID   string
	ProjectName string
	UserID      string
	Repo        string
	IsActive    bool
	RunCtx      *runtimepkg.ProjectRuntime
}
