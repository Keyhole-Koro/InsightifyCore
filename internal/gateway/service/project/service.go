package project

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"insightify/internal/gateway/entity"
	artifactrepo "insightify/internal/gateway/repository/artifact"
	projectrepo "insightify/internal/gateway/repository/project"
	runtimepkg "insightify/internal/workerruntime"
)

// Service implements Project business logic and owns all project state.
type Service struct {
	repo     projectrepo.Repository
	metaRepo projectrepo.ArtifactRepository
	artifact artifactrepo.Store

	runCtxMu sync.RWMutex
	runCtx   map[string]*runtimepkg.ProjectRuntime
}

// New creates a project service backed by the given store.
func New(repo projectrepo.Repository, metaRepo projectrepo.ArtifactRepository, artifact artifactrepo.Store) *Service {
	return &Service{
		repo:     repo,
		metaRepo: metaRepo,
		artifact: artifact,
		runCtx:   make(map[string]*runtimepkg.ProjectRuntime),
	}
}

// ---------------------------------------------------------------------------
// Business Logic
// ---------------------------------------------------------------------------

type ArtifactView struct {
	ID        string
	RunID     string
	Path      string
	URL       string
	CreatedAt time.Time
}

// Entry is the public type for project entry (was unexported 'entry').
type Entry struct {
	State     State
	RunCtx    *runtimepkg.ProjectRuntime
	Artifacts []ArtifactView
}

func (s *Service) ListProjects(ctx context.Context, userID entity.UserID) ([]Entry, string, error) {
	ctx = ensureContext(ctx)
	s.repo.EnsureLoaded(ctx)

	projects, err := s.listByUser(ctx, userID)
	if err != nil {
		return nil, "", err
	}
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

func (s *Service) CreateProject(ctx context.Context, userID entity.UserID, projectName string) (Entry, error) {
	repoCtx := ensureContext(ctx)
	s.repo.EnsureLoaded(repoCtx)

	if projectName == "" {
		projectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}

	projectID := fmt.Sprintf("project-%d", time.Now().UnixNano())

	var runCtx *runtimepkg.ProjectRuntime
	createdRunCtx, err := runtimepkg.NewProjectRuntime("", projectID)
	if err != nil {
		return Entry{}, fmt.Errorf("failed to create run context: %w", err)
	}
	runCtx = createdRunCtx

	p := Entry{
		State: State{
			ProjectID:   projectID,
			ProjectName: projectName,
			UserID:      userID,
			Repo:        "",
			IsActive:    true,
		},
		RunCtx: runCtx,
	}
	s.put(repoCtx, p)
	_, _ = s.setActiveForUser(repoCtx, userID, projectID)
	_ = s.repo.Save(repoCtx)

	got, _ := s.get(repoCtx, projectID)
	return got, nil
}

func (s *Service) SelectProject(ctx context.Context, userID entity.UserID, projectID string) (Entry, error) {
	ctx = ensureContext(ctx)
	s.repo.EnsureLoaded(ctx)

	p, ok := s.get(ctx, projectID)
	if !ok {
		return Entry{}, fmt.Errorf("project %s not found", projectID)
	}
	if p.State.UserID != userID {
		return Entry{}, fmt.Errorf("project %s does not belong to user %s", projectID, userID.String())
	}

	selected, ok := s.setActiveForUser(ctx, userID, projectID)
	if !ok {
		return Entry{}, fmt.Errorf("project %s not found", projectID)
	}
	_ = s.repo.Save(ctx)
	return selected, nil
}

func (s *Service) EnsureProject(ctx context.Context, userID entity.UserID, projectID string) (Entry, error) {
	ctx = ensureContext(ctx)
	s.repo.EnsureLoaded(ctx)

	if userID.IsZero() {
		userID = entity.DemoUserID
	}

	var (
		p       Entry
		existed bool
	)

	// Resolve project.
	if projectID == "" {
		if active, ok := s.getActiveByUser(ctx, userID); ok {
			projectID = active.State.ProjectID
		}
	}
	if projectID != "" {
		p, existed = s.get(ctx, projectID)
	}
	if !existed {
		if projectID == "" {
			projectID = fmt.Sprintf("project-%d", time.Now().UnixNano())
		}
		p = Entry{
			State: State{
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

	s.put(ctx, p)
	_, _ = s.setActiveForUser(ctx, p.State.UserID, p.State.ProjectID)
	_ = s.repo.Save(ctx)

	got, _ := s.get(ctx, projectID)
	return got, nil
}

// ---------------------------------------------------------------------------
// State management (absorbed from runtime.App)
// ---------------------------------------------------------------------------

func (s *Service) get(ctx context.Context, projectID string) (Entry, bool) {
	repoState, ok := s.repo.Get(ctx, projectID)
	if !ok {
		return Entry{}, false
	}
	s.runCtxMu.RLock()
	runCtx := s.runCtx[strings.TrimSpace(projectID)]
	s.runCtxMu.RUnlock()

	artifacts := s.resolveArtifacts(ctx, projectID)
	return Entry{State: fromRepoState(repoState), RunCtx: runCtx, Artifacts: artifacts}, true
}

func (s *Service) resolveArtifacts(ctx context.Context, projectID string) []ArtifactView {
	if s.artifact == nil || s.metaRepo == nil {
		return nil
	}
	list, err := s.metaRepo.ListArtifacts(ctx, projectID)
	if err != nil {
		// Log error? For now silence it to avoid disrupting main flow
		return nil
	}
	out := make([]ArtifactView, 0, len(list))
	for _, a := range list {
		url, _ := s.artifact.GetURL(ctx, a.RunID, a.Path)
		// ID is int in DB, converting to string for View/Proto
		out = append(out, ArtifactView{
			ID:        fmt.Sprintf("%d", a.ID),
			RunID:     a.RunID,
			Path:      a.Path,
			URL:       url,
			CreatedAt: a.CreatedAt,
		})
	}
	return out
}

func (s *Service) put(ctx context.Context, e Entry) {
	if strings.TrimSpace(e.State.ProjectID) == "" {
		return
	}
	_ = s.repo.Put(ensureContext(ctx), toRepoState(e.State))
	s.runCtxMu.Lock()
	s.runCtx[e.State.ProjectID] = e.RunCtx
	s.runCtxMu.Unlock()
}

func (s *Service) listByUser(ctx context.Context, userID entity.UserID) ([]Entry, error) {
	states, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(states))
	s.runCtxMu.RLock()
	for _, st := range states {
		state := fromRepoState(st)
		if !isProjectID(state.ProjectID) {
			continue
		}
		artifacts := s.resolveArtifacts(ctx, state.ProjectID)
		out = append(out, Entry{State: state, RunCtx: s.runCtx[state.ProjectID], Artifacts: artifacts})
	}
	s.runCtxMu.RUnlock()
	return out, nil
}

func (s *Service) getActiveByUser(ctx context.Context, userID entity.UserID) (Entry, bool) {
	ctx = ensureContext(ctx)
	st, ok, err := s.repo.GetActiveByUser(ctx, userID)
	if err != nil {
		return Entry{}, false
	}
	if !ok {
		return Entry{}, false
	}
	s.runCtxMu.RLock()
	state := fromRepoState(st)
	runCtx := s.runCtx[state.ProjectID]
	s.runCtxMu.RUnlock()

	artifacts := s.resolveArtifacts(ctx, state.ProjectID)
	return Entry{State: state, RunCtx: runCtx, Artifacts: artifacts}, true
}

func (s *Service) setActiveForUser(ctx context.Context, userID entity.UserID, projectID string) (Entry, bool) {
	st, ok, err := s.repo.SetActiveForUser(ctx, userID, projectID)
	if err != nil {
		return Entry{}, false
	}
	if !ok {
		return Entry{}, false
	}
	s.runCtxMu.RLock()
	state := fromRepoState(st)
	runCtx := s.runCtx[state.ProjectID]
	s.runCtxMu.RUnlock()

	artifacts := s.resolveArtifacts(ctx, state.ProjectID)
	return Entry{State: state, RunCtx: runCtx, Artifacts: artifacts}, true
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
	e, ok := s.get(context.Background(), projectID)
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
	e, ok := s.get(context.Background(), projectID)
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
	s.put(context.Background(), e)
	return ctx, nil
}

func ensureContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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
	UserID      entity.UserID
	Repo        string
	IsActive    bool
	RunCtx      *runtimepkg.ProjectRuntime
}

func fromRepoState(s projectrepo.State) State {
	return State{
		ProjectID:   s.ProjectID,
		ProjectName: s.ProjectName,
		UserID:      s.UserID,
		Repo:        s.Repo,
		IsActive:    s.IsActive,
	}
}

func toRepoState(s State) projectrepo.State {
	return projectrepo.State{
		ProjectID:   s.ProjectID,
		ProjectName: s.ProjectName,
		UserID:      s.UserID,
		Repo:        s.Repo,
		IsActive:    s.IsActive,
	}
}
