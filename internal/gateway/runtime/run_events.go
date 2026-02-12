package runtime

import (
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

const completedRunRetention = 30 * time.Second

func (a *App) AllocateRunEventChannel(runID string, size int) chan *insightifyv1.WatchRunResponse {
	if size <= 0 {
		size = 1
	}
	ch := make(chan *insightifyv1.WatchRunResponse, size)
	if a == nil {
		return ch
	}
	a.runMu.Lock()
	a.runEvents[strings.TrimSpace(runID)] = ch
	a.runMu.Unlock()
	return ch
}

func (a *App) RunEventChannel(runID string) (chan *insightifyv1.WatchRunResponse, bool) {
	if a == nil {
		return nil, false
	}
	a.runMu.RLock()
	ch, ok := a.runEvents[strings.TrimSpace(runID)]
	a.runMu.RUnlock()
	return ch, ok
}

func (a *App) ScheduleRunCleanup(runID string) {
	if a == nil {
		return
	}
	time.AfterFunc(completedRunRetention, func() {
		a.runMu.Lock()
		delete(a.runEvents, strings.TrimSpace(runID))
		a.runMu.Unlock()
	})
}
