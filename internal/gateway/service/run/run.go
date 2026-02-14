package run

import (
	"insightify/internal/gateway/repository/ui"
	"sync"
	"time"
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

// Service manages runs and traces.
type Service struct {
	project ProjectReader
	ui      *ui.Store
	trace   *TraceLogger
}

func New(project ProjectReader, ui *ui.Store) *Service {
	return &Service{
		project: project,
		ui:      ui,
		trace:   NewTraceLogger(),
	}
}

func (s *Service) TraceLogger() *TraceLogger {
	return s.trace
}

// ---------------------------------------------------------------------------
// Trace Logger
// ---------------------------------------------------------------------------

type TraceLogger struct {
	mu     sync.RWMutex
	events map[string][]map[string]any
}

func NewTraceLogger() *TraceLogger {
	return &TraceLogger{
		events: make(map[string][]map[string]any),
	}
}

func (l *TraceLogger) Append(runID, source, stage string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if fields == nil {
		fields = make(map[string]any)
	}
	// Copy fields to avoid mutation
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

func (l *TraceLogger) Read(runID string) ([]map[string]any, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	events, ok := l.events[runID]
	if !ok {
		return []map[string]any{}, nil
	}
	// Return copy
	out := make([]map[string]any, len(events))
	copy(out, events)
	return out, nil
}
