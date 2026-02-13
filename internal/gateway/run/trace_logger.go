package run

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var traceRunIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// TraceEvent is a structured run trace event persisted as JSON.
type TraceEvent struct {
	Timestamp string         `json:"timestamp"`
	RunID     string         `json:"run_id"`
	Source    string         `json:"source"`
	Stage     string         `json:"stage"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// TraceLogger persists run-scoped trace events into JSONL files.
type TraceLogger struct {
	dir string
	mu  sync.Mutex
}

func defaultRunTraceDir() string {
	return filepath.Join("tmp", "run_logs")
}

func newTraceLogger(dir string) *TraceLogger {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		trimmed = defaultRunTraceDir()
	}
	_ = os.MkdirAll(trimmed, 0o755)
	return &TraceLogger{dir: trimmed}
}

func sanitizeRunID(runID string) string {
	id := strings.TrimSpace(runID)
	if id == "" {
		return "unknown"
	}
	id = traceRunIDSanitizer.ReplaceAllString(id, "_")
	if id == "" {
		return "unknown"
	}
	return id
}

func (l *TraceLogger) filePath(runID string) string {
	return filepath.Join(l.dir, sanitizeRunID(runID)+".jsonl")
}

// Append writes one trace line for the run.
func (l *TraceLogger) Append(runID, source, stage string, fields map[string]any) {
	if l == nil || strings.TrimSpace(runID) == "" {
		return
	}
	event := TraceEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		RunID:     strings.TrimSpace(runID),
		Source:    strings.TrimSpace(source),
		Stage:     strings.TrimSpace(stage),
	}
	if len(fields) > 0 {
		event.Fields = fields
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	raw = append(raw, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_ = os.MkdirAll(l.dir, 0o755)
	f, err := os.OpenFile(l.filePath(runID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(raw)
}

// Read returns all persisted trace events for a run.
func (l *TraceLogger) Read(runID string) ([]TraceEvent, error) {
	if l == nil {
		return nil, nil
	}
	path := l.filePath(runID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []TraceEvent{}, nil
		}
		return nil, fmt.Errorf("open trace file: %w", err)
	}
	defer f.Close()

	out := make([]TraceEvent, 0, 64)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev TraceEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan trace file: %w", err)
	}
	return out, nil
}
