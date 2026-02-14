package worker

import (
	"sync"
	"time"
)

// TelemetryStore (formerly TraceLogger) stores run execution traces.
type TelemetryStore struct {
	mu     sync.RWMutex
	events map[string][]map[string]any
}

func NewTelemetryStore() *TelemetryStore {
	return &TelemetryStore{events: make(map[string][]map[string]any)}
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
	l.events[runID] = append(l.events[runID], evt)
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
