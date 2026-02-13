package run

import (
	"fmt"
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
	mu    sync.RWMutex
	byRun map[string]*pendingInput

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
func NewInteraction() *Interaction {
	return &Interaction{
		byRun:           make(map[string]*pendingInput),
		activeRunByProj: make(map[string]string),
		projectByRun:    make(map[string]string),
		runToConv:       make(map[string]string),
		convToRun:       make(map[string]string),
	}
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
		return "", fmt.Errorf("invalid pending input registration")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byRun[runID]; exists {
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
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-pending.done:
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case value := <-pending.inputCh:
		m.Clear(runID)
		return strings.TrimSpace(value), nil
	case <-timer.C:
		m.Clear(runID)
		return "", fmt.Errorf("run %s input wait timed out", runID)
	}
}

func (m *Interaction) SubmitUserInput(projectID, runID, interactionID, input string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	interactionID = strings.TrimSpace(interactionID)
	input = strings.TrimSpace(input)
	if projectID == "" || runID == "" {
		return "", fmt.Errorf("project_id and run_id are required")
	}
	if input == "" {
		return "", fmt.Errorf("input is required")
	}

	m.mu.RLock()
	pending, ok := m.byRun[runID]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}
	if pending.ProjectID != projectID {
		return "", fmt.Errorf("run %s does not belong to project %s", runID, projectID)
	}
	if interactionID != "" && pending.InteractionID != interactionID {
		interactionID = pending.InteractionID
	}

	select {
	case <-pending.done:
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case pending.inputCh <- input:
		return pending.InteractionID, nil
	default:
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
