package worker

import (
	artifactrepo "insightify/internal/gateway/repository/artifact"
	gatewayui "insightify/internal/gateway/service/ui"
	"insightify/internal/runner"
	runtimepkg "insightify/internal/workerruntime"
	"sync"
	"sync/atomic"
)

// ProjectReader is an interface to read project state without circular dependency on project service.
type ProjectReader interface {
	GetEntry(projectID string) (ProjectView, bool)
	EnsureRunContext(projectID string) (*runtimepkg.ProjectRuntime, error)
}

type WorkspaceRunBinder interface {
	AssignRunToCurrentTab(projectID, runID string) error
}

// ProjectView is a simplified view of a project.
type ProjectView struct {
	ProjectID string
	RunCtx    *runtimepkg.ProjectRuntime
}

// Service manages runs and telemetry.
type Service struct {
	project     ProjectReader
	workspaces  WorkspaceRunBinder
	ui          *gatewayui.Service
	interaction runner.InteractionWaiter
	artifact    artifactrepo.Store
	telemetry   *TelemetryStore

	runMu      sync.RWMutex
	runs       map[string]*WorkerRuntime
	runCounter atomic.Uint64
}

func New(project ProjectReader, workspaces WorkspaceRunBinder, ui *gatewayui.Service, interaction runner.InteractionWaiter, artifact artifactrepo.Store) *Service {
	return &Service{
		project:     project,
		workspaces:  workspaces,
		ui:          ui,
		interaction: interaction,
		artifact:    artifact,
		telemetry:   NewTelemetryStore(),
		runs:        make(map[string]*WorkerRuntime),
	}
}

func (s *Service) Telemetry() *TelemetryStore {
	return s.telemetry
}
