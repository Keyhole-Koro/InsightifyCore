package runtime

import (
	"sync"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/projectstore"
	"insightify/internal/gateway/ui"
	"insightify/internal/gateway/userinteraction"
	"insightify/internal/runner"
)

// RunEnvironment abstracts the worker execution environment.
// Implemented by handler.RunContext.
type RunEnvironment interface {
	GetEnv() *runner.Env
	GetOutDir() string
	GetID() string
}

// Project pairs persistent project state with an optional run environment.
type Project struct {
	State  projectstore.State
	RunCtx RunEnvironment
}

// App is the central runtime kernel of the gateway, owning concurrency-safe
// state for run events, run contexts, and UI nodes.
type App struct {
	projectStore *projectstore.Store
	interaction  *userinteraction.Manager
	uiStore      *ui.Store

	runMu     sync.RWMutex
	runEvents map[string]chan *insightifyv1.WatchRunResponse

	runCtxMu sync.RWMutex
	runCtx   map[string]RunEnvironment
}

func New(projectStore *projectstore.Store) *App {
	return &App{
		projectStore: projectStore,
		interaction:  userinteraction.New(),
		uiStore:      ui.NewStore(),
		runEvents:    make(map[string]chan *insightifyv1.WatchRunResponse),
		runCtx:       make(map[string]RunEnvironment),
	}
}

// Interaction returns the user-interaction manager, which handles
// run lifecycle, conversation state, and pending-input coordination.
func (a *App) Interaction() *userinteraction.Manager { return a.interaction }

// ProjectStore returns the underlying project persistence store.
func (a *App) ProjectStore() *projectstore.Store { return a.projectStore }

// SetRunNode updates the UI node for a run.
func (a *App) SetRunNode(runID string, node *insightifyv1.UiNode) {
	a.uiStore.Set(runID, node)
}

// GetRunNode returns the current UI node for a run.
func (a *App) GetRunNode(runID string) *insightifyv1.UiNode {
	return a.uiStore.Get(runID)
}

// ClearRunNode removes the UI node for a run.
func (a *App) ClearRunNode(runID string) {
	a.uiStore.Clear(runID)
}
