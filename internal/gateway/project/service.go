package project

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/projectstore"
	"insightify/internal/gateway/runtime"

	"connectrpc.com/connect"
)

// Service implements ProjectServiceHandler and owns all project state.
type Service struct {
	store *projectstore.Store

	runCtxMu sync.RWMutex
	runCtx   map[string]runtime.RunEnvironment
}

// New creates a project service backed by the given store.
func New(store *projectstore.Store) *Service {
	return &Service{
		store:  store,
		runCtx: make(map[string]runtime.RunEnvironment),
	}
}

// Store returns the underlying project persistence store.
func (s *Service) Store() *projectstore.Store { return s.store }

// ---------------------------------------------------------------------------
// ProjectServiceHandler RPC implementations
// ---------------------------------------------------------------------------

func (s *Service) ListProjects(_ context.Context, req *connect.Request[insightifyv1.ListProjectsRequest]) (*connect.Response[insightifyv1.ListProjectsResponse], error) {
	s.store.EnsureLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}

	projects := s.listByUser(userID)
	sort.Slice(projects, func(i, j int) bool {
		return strings.TrimSpace(projects[i].State.ProjectID) < strings.TrimSpace(projects[j].State.ProjectID)
	})

	out := &insightifyv1.ListProjectsResponse{Projects: make([]*insightifyv1.Project, 0, len(projects))}
	for _, p := range projects {
		pp := toProtoProject(p)
		out.Projects = append(out.Projects, pp)
		if pp.GetIsActive() && out.GetActiveProjectId() == "" {
			out.ActiveProjectId = pp.GetProjectId()
		}
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateProject(_ context.Context, req *connect.Request[insightifyv1.CreateProjectRequest]) (*connect.Response[insightifyv1.CreateProjectResponse], error) {
	s.store.EnsureLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id is required"))
	}
	projectName := strings.TrimSpace(req.Msg.GetName())
	if projectName == "" {
		projectName = fmt.Sprintf("Project %d", time.Now().Unix()%100000)
	}

	projectID := fmt.Sprintf("project-%d", time.Now().UnixNano())

	var runCtx runtime.RunEnvironment
	ctx, err := runtime.NewRunContext("", projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
	}
	runCtx = ctx

	p := entry{
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
	return connect.NewResponse(&insightifyv1.CreateProjectResponse{Project: toProtoProject(got)}), nil
}

func (s *Service) SelectProject(_ context.Context, req *connect.Request[insightifyv1.SelectProjectRequest]) (*connect.Response[insightifyv1.SelectProjectResponse], error) {
	s.store.EnsureLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	projectID := strings.TrimSpace(req.Msg.GetProjectId())
	if userID == "" || projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and project_id are required"))
	}

	p, ok := s.get(projectID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	if strings.TrimSpace(p.State.UserID) != userID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("project %s does not belong to user %s", projectID, userID))
	}

	selected, ok := s.setActiveForUser(userID, projectID)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project %s not found", projectID))
	}
	s.store.Save()
	return connect.NewResponse(&insightifyv1.SelectProjectResponse{Project: toProtoProject(selected)}), nil
}

func (s *Service) InitRun(_ context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	s.store.EnsureLoaded()
	userID := strings.TrimSpace(req.Msg.GetUserId())
	if userID == "" {
		userID = "demo-user"
	}
	projectID := strings.TrimSpace(req.Msg.GetProjectId())

	var (
		p       entry
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
		p = entry{
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
		ctx, err := runtime.NewRunContext(p.State.Repo, projectID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create run context: %w", err))
		}
		p.RunCtx = ctx
	}

	s.put(p)
	s.setActiveForUser(p.State.UserID, p.State.ProjectID)
	s.store.Save()

	got, _ := s.get(projectID)
	return connect.NewResponse(&insightifyv1.InitRunResponse{
		RepoName:       got.State.Repo,
		BootstrapRunId: "",
		ProjectId:      got.State.ProjectID,
	}), nil
}

// ---------------------------------------------------------------------------
// State management (absorbed from runtime.App)
// ---------------------------------------------------------------------------

// entry pairs project state with an optional run environment.
type entry struct {
	State  projectstore.State
	RunCtx runtime.RunEnvironment
}

func (s *Service) get(projectID string) (entry, bool) {
	state, ok := s.store.Get(projectID)
	if !ok {
		return entry{}, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[strings.TrimSpace(projectID)]
	s.runCtxMu.RUnlock()
	return entry{State: state, RunCtx: ctx}, true
}

func (s *Service) put(e entry) {
	if strings.TrimSpace(e.State.ProjectID) == "" {
		return
	}
	s.store.Put(e.State)
	s.runCtxMu.Lock()
	s.runCtx[e.State.ProjectID] = e.RunCtx
	s.runCtxMu.Unlock()
}

func (s *Service) listByUser(userID string) []entry {
	states := s.store.ListByUser(userID)
	out := make([]entry, 0, len(states))
	s.runCtxMu.RLock()
	for _, st := range states {
		if !isProjectID(st.ProjectID) {
			continue
		}
		out = append(out, entry{State: st, RunCtx: s.runCtx[st.ProjectID]})
	}
	s.runCtxMu.RUnlock()
	return out
}

func (s *Service) getActiveByUser(userID string) (entry, bool) {
	st, ok := s.store.GetActiveByUser(userID)
	if !ok {
		return entry{}, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[st.ProjectID]
	s.runCtxMu.RUnlock()
	return entry{State: st, RunCtx: ctx}, true
}

func (s *Service) setActiveForUser(userID, projectID string) (entry, bool) {
	st, ok := s.store.SetActiveForUser(userID, projectID)
	if !ok {
		return entry{}, false
	}
	s.runCtxMu.RLock()
	ctx := s.runCtx[st.ProjectID]
	s.runCtxMu.RUnlock()
	return entry{State: st, RunCtx: ctx}, true
}

// ---------------------------------------------------------------------------
// Public accessors (used by run.Service via ProjectReader interface)
// ---------------------------------------------------------------------------

// GetRunContext returns the run context for a project.
func (s *Service) GetRunContext(projectID string) (runtime.RunEnvironment, bool) {
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
func (s *Service) EnsureRunContext(projectID string) (runtime.RunEnvironment, error) {
	e, ok := s.get(projectID)
	if !ok {
		return nil, fmt.Errorf("project %s not found", projectID)
	}
	if e.RunCtx != nil && s.hasRequiredWorkers(e.RunCtx) {
		return e.RunCtx, nil
	}
	ctx, err := runtime.NewRunContext(e.State.Repo, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to restore run context: %w", err)
	}
	e.RunCtx = ctx
	s.put(e)
	return ctx, nil
}

func (s *Service) hasRequiredWorkers(env runtime.RunEnvironment) bool {
	if env == nil || env.GetEnv() == nil || env.GetEnv().Resolver == nil {
		return false
	}
	_, hasBootstrap := env.GetEnv().Resolver.Get("bootstrap")
	_, hasTestLLM := env.GetEnv().Resolver.Get("testllmChar")
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
	RunCtx      runtime.RunEnvironment
}
