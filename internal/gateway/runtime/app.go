package runtime

import (
	"sync"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/projectstore"
	"insightify/internal/gateway/userinteraction"
)

type Project struct {
	State  projectstore.State
	RunCtx any
}

type App struct {
	projectStore *projectstore.Store
	interaction  *userinteraction.Manager

	runMu     sync.RWMutex
	runEvents map[string]chan *insightifyv1.WatchRunResponse

	runCtxMu sync.RWMutex
	runCtx   map[string]any

	runNodeMu sync.RWMutex
	runNodes  map[string]*insightifyv1.UiNode
}

func New(projectStore *projectstore.Store) *App {
	return &App{
		projectStore: projectStore,
		interaction:  userinteraction.New(),
		runEvents:    make(map[string]chan *insightifyv1.WatchRunResponse),
		runCtx:       make(map[string]any),
		runNodes:     make(map[string]*insightifyv1.UiNode),
	}
}
