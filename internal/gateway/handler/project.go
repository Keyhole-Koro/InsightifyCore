package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/artifact"
	"insightify/internal/gateway/projectstore"
	"insightify/internal/gateway/runtime"
	projectuc "insightify/internal/gateway/usecase/project"

	"connectrpc.com/connect"
)

func (s *Service) ListProjects(_ context.Context, req *connect.Request[insightifyv1.ListProjectsRequest]) (*connect.Response[insightifyv1.ListProjectsResponse], error) {
	projects, err := projectuc.List(req.Msg.GetUserId(), s.projectDeps())
	if err != nil {
		return nil, err
	}
	sort.Slice(projects, func(i, j int) bool {
		return strings.TrimSpace(projects[i].ProjectID) < strings.TrimSpace(projects[j].ProjectID)
	})
	out := &insightifyv1.ListProjectsResponse{Projects: make([]*insightifyv1.Project, 0, len(projects))}
	for _, p := range projects {
		pp := toProtoProject(projectStateFromUsecase(p))
		out.Projects = append(out.Projects, pp)
		if pp.GetIsActive() && out.GetActiveProjectId() == "" {
			out.ActiveProjectId = pp.GetProjectId()
		}
	}
	return connect.NewResponse(out), nil
}

func (s *Service) CreateProject(_ context.Context, req *connect.Request[insightifyv1.CreateProjectRequest]) (*connect.Response[insightifyv1.CreateProjectResponse], error) {
	created, err := projectuc.Create(req.Msg.GetUserId(), req.Msg.GetName(), time.Now(), s.projectDeps())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&insightifyv1.CreateProjectResponse{Project: toProtoProject(projectStateFromUsecase(created))}), nil
}

func (s *Service) SelectProject(_ context.Context, req *connect.Request[insightifyv1.SelectProjectRequest]) (*connect.Response[insightifyv1.SelectProjectResponse], error) {
	selected, err := projectuc.Select(req.Msg.GetUserId(), req.Msg.GetProjectId(), s.projectDeps())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&insightifyv1.SelectProjectResponse{Project: toProtoProject(projectStateFromUsecase(selected))}), nil
}

func (s *Service) InitRun(_ context.Context, req *connect.Request[insightifyv1.InitRunRequest]) (*connect.Response[insightifyv1.InitRunResponse], error) {
	in := s.prepareInitRun(req)
	updated, err := projectuc.InitRun(
		projectuc.InitRunInput{ProjectID: in.ProjectID, UserID: in.UserID, RepoURL: in.RepoURL},
		time.Now(),
		s.projectDeps(),
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&insightifyv1.InitRunResponse{
		RepoName:       updated.Repo,
		BootstrapRunId: "",
		ProjectId:      updated.ProjectID,
	}), nil
}

// ---------------------------------------------------------------------------
// project deps wiring
// ---------------------------------------------------------------------------

func (s *Service) projectDeps() projectuc.Deps {
	return projectuc.Deps{
		EnsureLoaded: s.app.ProjectStore().EnsureLoaded,
		Persist:      s.app.ProjectStore().Save,
		GetState: func(projectID string) (projectuc.Session, bool) {
			p, ok := s.app.GetProject(projectID)
			if !ok {
				return projectuc.Session{}, false
			}
			return projectToSession(p), true
		},
		PutState: func(sess projectuc.Session) {
			s.app.PutProject(sessionToProject(sess))
		},
		ListByUser: func(userID string) []projectuc.Session {
			list := s.listProjectsByUser(userID)
			out := make([]projectuc.Session, 0, len(list))
			for _, ps := range list {
				out = append(out, projectStateToSession(ps))
			}
			return out
		},
		GetActiveByUser: func(userID string) (projectuc.Session, bool) {
			ps, ok := s.getActiveProjectByUser(userID)
			if !ok {
				return projectuc.Session{}, false
			}
			return projectStateToSession(ps), true
		},
		SetActiveForUser: func(userID, projectID string) (projectuc.Session, bool) {
			ps, ok := s.setActiveProjectForUser(userID, projectID)
			if !ok {
				return projectuc.Session{}, false
			}
			return projectStateToSession(ps), true
		},
		NewRunContext: func(repo, projectID string) (runtime.RunEnvironment, error) {
			return NewRunContext(repo, projectID)
		},
		HasRequiredWorkers: hasRequiredWorkersEnv,
	}
}

// ---------------------------------------------------------------------------
// project state helpers – direct s.app access, no globals
// ---------------------------------------------------------------------------

type projectState struct {
	projectstore.State
	RunCtx *RunContext
}

func (s *Service) getProjectState(projectID string) (projectState, bool) {
	p, ok := s.app.GetProject(projectID)
	if !ok {
		return projectState{}, false
	}
	runCtx, _ := p.RunCtx.(*RunContext) // handler-specific downcast
	return projectState{State: p.State, RunCtx: runCtx}, true
}

func (s *Service) putProjectState(ps projectState) {
	if strings.TrimSpace(ps.ProjectID) == "" {
		return
	}
	s.app.PutProject(runtime.Project{State: ps.State, RunCtx: ps.RunCtx}) // *RunContext implements RunEnvironment
}

func (s *Service) updateProjectState(projectID string, update func(*projectState)) (projectState, bool) {
	cur, ok := s.getProjectState(projectID)
	if !ok {
		return projectState{}, false
	}
	update(&cur)
	s.putProjectState(cur)
	return cur, true
}

func (s *Service) listProjectsByUser(userID string) []projectState {
	records := s.app.ListProjectsByUser(userID)
	projects := make([]projectState, 0, len(records))
	for _, r := range records {
		if !isProjectID(r.State.ProjectID) {
			continue
		}
		runCtx, _ := r.RunCtx.(*RunContext) // handler-specific downcast
		projects = append(projects, projectState{State: r.State, RunCtx: runCtx})
	}
	return projects
}

func (s *Service) getActiveProjectByUser(userID string) (projectState, bool) {
	for _, p := range s.listProjectsByUser(userID) {
		if p.IsActive {
			return p, true
		}
	}
	return projectState{}, false
}

func (s *Service) setActiveProjectForUser(userID, projectID string) (projectState, bool) {
	r, ok := s.app.SetActiveProjectForUser(userID, projectID)
	if !ok {
		return projectState{}, false
	}
	if !isProjectID(r.State.ProjectID) {
		return projectState{}, false
	}
	runCtx, _ := r.RunCtx.(*RunContext) // handler-specific downcast
	return projectState{State: r.State, RunCtx: runCtx}, true
}

func (s *Service) ensureProjectRunContext(projectID string) (projectState, error) {
	sess, ok := s.getProjectState(projectID)
	if !ok {
		return projectState{}, fmt.Errorf("project %s not found", projectID)
	}
	if sess.RunCtx != nil && hasRequiredWorkers(sess.RunCtx) {
		return sess, nil
	}
	runCtx, err := NewRunContext(sess.Repo, projectID)
	if err != nil {
		return projectState{}, fmt.Errorf("failed to restore run context: %w", err)
	}
	updated, _ := s.updateProjectState(projectID, func(cur *projectState) { cur.RunCtx = runCtx })
	return updated, nil
}

func hasRequiredWorkers(runCtx *RunContext) bool {
	if runCtx == nil || runCtx.Env == nil || runCtx.Env.Resolver == nil {
		return false
	}
	_, hasBootstrap := runCtx.Env.Resolver.Get("bootstrap")
	_, hasTestLLM := runCtx.Env.Resolver.Get("testllmChar")
	return hasBootstrap && hasTestLLM
}

// hasRequiredWorkersEnv checks via the runtime.RunEnvironment interface,
// used by usecase Deps that don't know about *RunContext.
func hasRequiredWorkersEnv(env runtime.RunEnvironment) bool {
	if env == nil || env.GetEnv() == nil || env.GetEnv().Resolver == nil {
		return false
	}
	_, hasBootstrap := env.GetEnv().Resolver.Get("bootstrap")
	_, hasTestLLM := env.GetEnv().Resolver.Get("testllmChar")
	return hasBootstrap && hasTestLLM
}

// ---------------------------------------------------------------------------
// proto / usecase conversion helpers
// ---------------------------------------------------------------------------

func projectToSession(p runtime.Project) projectuc.Session {
	return projectuc.Session{
		ProjectID:   p.State.ProjectID,
		ProjectName: p.State.ProjectName,
		UserID:      p.State.UserID,
		Repo:        p.State.Repo,
		IsActive:    p.State.IsActive,
		RunCtx:      p.RunCtx,
	}
}

func sessionToProject(sess projectuc.Session) runtime.Project {
	runEnv, _ := sess.RunCtx.(runtime.RunEnvironment)
	return runtime.Project{
		State: projectstore.State{
			ProjectID:   strings.TrimSpace(sess.ProjectID),
			ProjectName: strings.TrimSpace(sess.ProjectName),
			UserID:      strings.TrimSpace(sess.UserID),
			Repo:        strings.TrimSpace(sess.Repo),
			IsActive:    sess.IsActive,
		},
		RunCtx: runEnv,
	}
}

func projectStateToSession(ps projectState) projectuc.Session {
	return projectuc.Session{
		ProjectID:   ps.ProjectID,
		ProjectName: ps.ProjectName,
		UserID:      ps.UserID,
		Repo:        ps.Repo,
		IsActive:    ps.IsActive,
		RunCtx:      ps.RunCtx,
	}
}

func projectStateFromUsecase(sess projectuc.Session) projectState {
	runCtx, _ := sess.RunCtx.(*RunContext)
	return projectState{
		State: projectstore.State{
			ProjectID:   strings.TrimSpace(sess.ProjectID),
			ProjectName: strings.TrimSpace(sess.ProjectName),
			UserID:      strings.TrimSpace(sess.UserID),
			Repo:        strings.TrimSpace(sess.Repo),
			IsActive:    sess.IsActive,
		},
		RunCtx: runCtx,
	}
}

func toProtoProject(ps projectState) *insightifyv1.Project {
	bc := readBootstrapContext(ps)
	projectID := strings.TrimSpace(ps.ProjectID)
	name := strings.TrimSpace(ps.ProjectName)
	if name == "" {
		name = "Project"
	}
	return &insightifyv1.Project{
		ProjectId: projectID,
		UserId:    strings.TrimSpace(ps.UserID),
		Name:      name,
		RepoUrl:   strings.TrimSpace(bc.RepoURL),
		Purpose:   strings.TrimSpace(bc.Purpose),
		RepoName:  strings.TrimSpace(ps.Repo),
		IsActive:  ps.IsActive,
	}
}

func readBootstrapContext(ps projectState) artifact.BootstrapContext {
	if ps.RunCtx == nil {
		return artifact.BootstrapContext{}
	}
	path := filepath.Join(ps.RunCtx.OutDir, "bootstrap.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return artifact.BootstrapContext{}
	}
	var raw struct {
		BootstrapContext artifact.BootstrapContext `json:"bootstrap_context"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return artifact.BootstrapContext{}
	}
	return raw.BootstrapContext.Normalize()
}
