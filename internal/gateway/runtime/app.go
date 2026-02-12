package runtime

import (
	"sync"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/projectstore"
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

	runMu     sync.RWMutex
	runEvents map[string]chan *insightifyv1.WatchRunResponse

	runCtxMu sync.RWMutex
	runCtx   map[string]RunEnvironment

	runNodeMu sync.RWMutex
	runNodes  map[string]*insightifyv1.UiNode
}

func New(projectStore *projectstore.Store) *App {
	return &App{
		projectStore: projectStore,
		interaction:  userinteraction.New(),
		runEvents:    make(map[string]chan *insightifyv1.WatchRunResponse),
		runCtx:       make(map[string]RunEnvironment),
		runNodes:     make(map[string]*insightifyv1.UiNode),
	}
}

// Interaction returns the user-interaction manager, which handles
// run lifecycle, conversation state, and pending-input coordination.
func (a *App) Interaction() *userinteraction.Manager { return a.interaction }

// ProjectStore returns the underlying project persistence store.
func (a *App) ProjectStore() *projectstore.Store { return a.projectStore }
