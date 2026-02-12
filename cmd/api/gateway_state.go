package main

import (
	"path/filepath"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/projectstore"
)

// runStore holds active runs and their event channels.
var runStore = struct {
	sync.RWMutex
	runs map[string]chan *insightifyv1.WatchRunResponse
}{
	runs: make(map[string]chan *insightifyv1.WatchRunResponse),
}

const completedRunRetention = 30 * time.Second

func scheduleRunCleanup(runID string) {
	time.AfterFunc(completedRunRetention, func() {
		runStore.Lock()
		delete(runStore.runs, runID)
		runStore.Unlock()
	})
}

type projectState struct {
	projectstore.State
	RunCtx *RunContext
}

var projectStateStore = projectstore.New(filepath.Join("tmp", "project_states.json"))

var runContextStore = struct {
	sync.RWMutex
	byProjectID map[string]*RunContext
}{
	byProjectID: make(map[string]*RunContext),
}
