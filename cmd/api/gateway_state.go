package main

import (
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

const sessionCookieName = "insightify_session_id"

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

type initSession struct {
	SessionID   string
	UserID      string
	RepoURL     string
	Purpose     string
	Repo        string
	RunCtx      *RunContext
	Running     bool
	ActiveRunID string
}

var initRunStore = struct {
	sync.RWMutex
	sessions map[string]initSession
}{
	sessions: make(map[string]initSession),
}
