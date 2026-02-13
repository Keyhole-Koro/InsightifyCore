package run

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// pendingInput represents a run waiting for user input.
type pendingInput struct {
	InteractionID string
	ProjectID     string
	RunID         string
	WorkerKey     string
	Prompt        string
	inputCh       chan string
	done          chan struct{}
	closeOnce     sync.Once
}

func (p *pendingInput) closeDone() {
	p.closeOnce.Do(func() {
		close(p.done)
	})
}

// PendingView is a read-only view of pending input state.
type PendingView struct {
	InteractionID string
	ProjectID     string
	RunID         string
	WorkerKey     string
	Prompt        string
}

// Interaction manages pending user input and run lifecycle tracking.
type Interaction struct {
	mu     sync.RWMutex
	byRun  map[string]*pendingInput
	tracer *TraceLogger

	// Run lifecycle tracking (active run per project).
	runMu           sync.RWMutex
	activeRunByProj map[string]string
	projectByRun    map[string]string

	// Conversation ID mapping.
	convMu    sync.RWMutex
	runToConv map[string]string
	convToRun map[string]string
}

// NewInteraction creates a new interaction manager.
func NewInteraction(tracer *TraceLogger) *Interaction {
	return &Interaction{
		byRun:           make(map[string]*pendingInput),
		tracer:          tracer,
		activeRunByProj: make(map[string]string),
		projectByRun:    make(map[string]string),
		runToConv:       make(map[string]string),
		convToRun:       make(map[string]string),
	}
}

func (m *Interaction) trace(runID, stage string, fields map[string]any) {
	if m == nil || m.tracer == nil {
		return
	}
	m.tracer.Append(runID, "interaction", stage, fields)
}

// ---------------------------------------------------------------------------
// Run lifecycle
// ---------------------------------------------------------------------------

func (m *Interaction) MarkRunStarted(projectID, runID string) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	if projectID == "" || runID == "" {
		return
	}
	m.runMu.Lock()
	m.activeRunByProj[projectID] = runID
	m.projectByRun[runID] = projectID
	m.runMu.Unlock()
}

func (m *Interaction) MarkRunFinished(projectID, runID string) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	if projectID == "" || runID == "" {
		return
	}
	m.runMu.Lock()
	if cur := strings.TrimSpace(m.activeRunByProj[projectID]); cur == runID {
		delete(m.activeRunByProj, projectID)
	}
	delete(m.projectByRun, runID)
	m.runMu.Unlock()
}

func (m *Interaction) ActiveRunID(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ""
	}
	m.runMu.RLock()
	runID := strings.TrimSpace(m.activeRunByProj[projectID])
	m.runMu.RUnlock()
	return runID
}

// ---------------------------------------------------------------------------
// Pending input
// ---------------------------------------------------------------------------

func (m *Interaction) RegisterNeedInput(projectID, runID, workerKey, prompt string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	workerKey = strings.TrimSpace(workerKey)
	if projectID == "" || runID == "" || workerKey == "" {
		log.Printf("[interaction] RegisterNeedInput invalid args: project_id=%s run_id=%s worker=%s", projectID, runID, workerKey)
		return "", fmt.Errorf("invalid pending input registration")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byRun[runID]; exists {
		log.Printf("[interaction] RegisterNeedInput already pending: project_id=%s run_id=%s worker=%s", projectID, runID, workerKey)
		m.trace(runID, "register_need_input_rejected", map[string]any{
			"project_id": projectID,
			"worker":     workerKey,
			"reason":     "already_pending",
		})
		return "", fmt.Errorf("run %s already waiting for user input", runID)
	}

	interactionID := fmt.Sprintf("input-%d", time.Now().UnixNano())
	m.byRun[runID] = &pendingInput{
		InteractionID: interactionID,
		ProjectID:     projectID,
		RunID:         runID,
		WorkerKey:     workerKey,
		Prompt:        strings.TrimSpace(prompt),
		inputCh:       make(chan string, 1),
		done:          make(chan struct{}),
	}
	log.Printf("[interaction] RegisterNeedInput ok: project_id=%s run_id=%s worker=%s interaction_id=%s prompt_len=%d", projectID, runID, workerKey, interactionID, len(strings.TrimSpace(prompt)))
	m.trace(runID, "register_need_input", map[string]any{
		"project_id":     projectID,
		"worker":         workerKey,
		"interaction_id": interactionID,
		"prompt_len":     len(strings.TrimSpace(prompt)),
	})
	return interactionID, nil
}

func (m *Interaction) WaitUserInput(runID string, timeout time.Duration) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}

	m.mu.RLock()
	pending, ok := m.byRun[runID]
	m.mu.RUnlock()
	if !ok {
		log.Printf("[interaction] WaitUserInput missing pending: run_id=%s", runID)
		m.trace(runID, "wait_missing_pending", nil)
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}
	log.Printf("[interaction] WaitUserInput begin: run_id=%s interaction_id=%s timeout=%s", runID, pending.InteractionID, timeout.String())
	m.trace(runID, "wait_begin", map[string]any{
		"interaction_id": pending.InteractionID,
		"timeout":        timeout.String(),
	})

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-pending.done:
		log.Printf("[interaction] WaitUserInput canceled: run_id=%s interaction_id=%s", runID, pending.InteractionID)
		m.trace(runID, "wait_canceled", map[string]any{
			"interaction_id": pending.InteractionID,
		})
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case value := <-pending.inputCh:
		m.Clear(runID)
		log.Printf("[interaction] WaitUserInput received: run_id=%s interaction_id=%s input_len=%d", runID, pending.InteractionID, len(strings.TrimSpace(value)))
		m.trace(runID, "wait_received", map[string]any{
			"interaction_id": pending.InteractionID,
			"input_len":      len(strings.TrimSpace(value)),
		})
		return strings.TrimSpace(value), nil
	case <-timer.C:
		m.Clear(runID)
		log.Printf("[interaction] WaitUserInput timeout: run_id=%s interaction_id=%s", runID, pending.InteractionID)
		m.trace(runID, "wait_timeout", map[string]any{
			"interaction_id": pending.InteractionID,
		})
		return "", fmt.Errorf("run %s input wait timed out", runID)
	}
}

func (m *Interaction) SubmitUserInput(projectID, runID, interactionID, input string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	interactionID = strings.TrimSpace(interactionID)
	input = strings.TrimSpace(input)
	if projectID == "" || runID == "" {
		log.Printf("[interaction] SubmitUserInput invalid args: project_id=%s run_id=%s", projectID, runID)
		m.trace(runID, "submit_invalid_args", map[string]any{
			"project_id": projectID,
		})
		return "", fmt.Errorf("project_id and run_id are required")
	}
	if input == "" {
		return "", fmt.Errorf("input is required")
	}

	m.mu.RLock()
	pending, ok := m.byRun[runID]
	m.mu.RUnlock()
	if !ok {
		log.Printf("[interaction] SubmitUserInput no pending: project_id=%s run_id=%s", projectID, runID)
		m.trace(runID, "submit_no_pending", map[string]any{
			"project_id": projectID,
		})
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}
	if pending.ProjectID != projectID {
		log.Printf("[interaction] SubmitUserInput project mismatch: project_id=%s run_id=%s pending_project=%s", projectID, runID, pending.ProjectID)
		m.trace(runID, "submit_project_mismatch", map[string]any{
			"project_id":         projectID,
			"pending_project_id": pending.ProjectID,
		})
		return "", fmt.Errorf("run %s does not belong to project %s", runID, projectID)
	}
	if interactionID != "" && pending.InteractionID != interactionID {
		log.Printf("[interaction] SubmitUserInput interaction mismatch: run_id=%s provided=%s expected=%s", runID, interactionID, pending.InteractionID)
		m.trace(runID, "submit_interaction_mismatch", map[string]any{
			"provided": interactionID,
			"expected": pending.InteractionID,
		})
		interactionID = pending.InteractionID
	}

	select {
	case <-pending.done:
		log.Printf("[interaction] SubmitUserInput canceled: run_id=%s interaction_id=%s", runID, pending.InteractionID)
		m.trace(runID, "submit_canceled", map[string]any{
			"interaction_id": pending.InteractionID,
		})
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case pending.inputCh <- input:
		log.Printf("[interaction] SubmitUserInput accepted: project_id=%s run_id=%s interaction_id=%s input_len=%d", projectID, runID, pending.InteractionID, len(input))
		m.trace(runID, "submit_accepted", map[string]any{
			"project_id":     projectID,
			"interaction_id": pending.InteractionID,
			"input_len":      len(input),
		})
		return pending.InteractionID, nil
	default:
		log.Printf("[interaction] SubmitUserInput channel full: run_id=%s interaction_id=%s", runID, pending.InteractionID)
		m.trace(runID, "submit_channel_full", map[string]any{
			"interaction_id": pending.InteractionID,
		})
		return "", fmt.Errorf("run %s already received pending input", runID)
	}
}

func (m *Interaction) Clear(runID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}

	m.mu.Lock()
	pending, ok := m.byRun[runID]
	if ok {
		delete(m.byRun, runID)
	}
	m.mu.Unlock()

	if ok {
		log.Printf("[interaction] Clear pending: run_id=%s interaction_id=%s", runID, pending.InteractionID)
		m.trace(runID, "clear_pending", map[string]any{
			"interaction_id": pending.InteractionID,
		})
		pending.closeDone()
	}
}

func (m *Interaction) GetPending(runID string) (PendingView, bool) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return PendingView{}, false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	pending, ok := m.byRun[runID]
	if !ok || pending == nil {
		return PendingView{}, false
	}
	return PendingView{
		InteractionID: pending.InteractionID,
		ProjectID:     pending.ProjectID,
		RunID:         pending.RunID,
		WorkerKey:     pending.WorkerKey,
		Prompt:        pending.Prompt,
	}, true
}

// ---------------------------------------------------------------------------
// Conversation ID mapping
// ---------------------------------------------------------------------------

func (m *Interaction) ConversationIDByRun(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	m.convMu.RLock()
	convID := m.runToConv[runID]
	m.convMu.RUnlock()
	if strings.TrimSpace(convID) != "" {
		return convID
	}
	return runID // default: use run ID as conversation ID
}

func (m *Interaction) EnsureConversation(runID, conversationID string) string {
	runID = strings.TrimSpace(runID)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = runID
	}
	if conversationID == "" {
		return ""
	}

	m.convMu.Lock()
	defer m.convMu.Unlock()
	if runID != "" {
		m.runToConv[runID] = conversationID
		m.convToRun[conversationID] = runID
	}
	return conversationID
}
