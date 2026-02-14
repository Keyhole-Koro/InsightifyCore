package worker

import (
	gatewayui "insightify/internal/gateway/service/ui"
	"insightify/internal/runner"
	"sync"
	"sync/atomic"
)

// ProjectReader is an interface to read project state without circular dependency on project service.
type ProjectReader interface {
	GetEntry(projectID string) (ProjectView, bool)
	EnsureRunContext(projectID string) (RunEnvironment, error)
}

// ProjectView is a simplified view of a project.
type ProjectView struct {
	ProjectID string
	RunCtx    RunEnvironment
}

// Service manages runs and telemetry.
type Service struct {
	project     ProjectReader
	ui          *gatewayui.Service
	interaction runner.InteractionWaiter
	telemetry   *TelemetryStore

	runMu      sync.RWMutex
	runs       map[string]*runState
	runCounter atomic.Uint64
}

func New(project ProjectReader, ui *gatewayui.Service, interaction runner.InteractionWaiter) *Service {
	return &Service{
		project:     project,
		ui:          ui,
		interaction: interaction,
		telemetry:   NewTelemetryStore(),
		runs:        make(map[string]*runState),
	}
}

func (s *Service) Telemetry() *TelemetryStore {
	return s.telemetry
}
