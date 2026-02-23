package worker

import (
	"sync"
	"time"
)

// TelemetryStore (formerly TraceLogger) stores run execution traces.
type TelemetryStore struct {
	mu     sync.RWMutex
	events map[string][]map[string]any
	order  []string
}

func NewTelemetryStore() *TelemetryStore {
	return &TelemetryStore{
		events: make(map[string][]map[string]any),
		order:  make([]string, 0, 32),
	}
}

func (l *TelemetryStore) Append(runID, source, stage string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if fields == nil {
		fields = map[string]any{}
	}
	evt := make(map[string]any, len(fields)+4)
	for k, v := range fields {
		evt[k] = v
	}
	evt["run_id"] = runID
	evt["source"] = source
	evt["stage"] = stage
	if _, ok := evt["timestamp"]; !ok {
		evt["timestamp"] = time.Now().Format(time.RFC3339Nano)
	}
	_, existed := l.events[runID]
	l.events[runID] = append(l.events[runID], evt)
	if !existed {
		l.order = append(l.order, runID)
		return
	}
	// Move existing runID to the end so LatestRuns reflects recent activity.
	for i := len(l.order) - 1; i >= 0; i-- {
		if l.order[i] != runID {
			continue
		}
		l.order = append(l.order[:i], l.order[i+1:]...)
		break
	}
	l.order = append(l.order, runID)
}

func (l *TelemetryStore) Read(runID string) ([]map[string]any, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	events, ok := l.events[runID]
	if !ok {
		return []map[string]any{}, nil
	}
	out := make([]map[string]any, len(events))
	copy(out, events)
	return out, nil
}

func (l *TelemetryStore) LatestRuns(limit int) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	if len(l.order) == 0 {
		return nil
	}
	if limit > len(l.order) {
		limit = len(l.order)
	}
	out := make([]string, 0, limit)
	for i := len(l.order) - 1; i >= 0 && len(out) < limit; i-- {
		runID := l.order[i]
		if runID == "" {
			continue
		}
		out = append(out, runID)
	}
	return out
}
