package llminteraction

import (
	"fmt"
	insightifyv1 "insightify/gen/go/insightify/v1"
	"strings"
	"sync"
	"time"
)

type pendingInput struct {
	InteractionID string
	SessionID     string
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

type PendingView struct {
	InteractionID string
	SessionID     string
	RunID         string
	WorkerKey     string
	Prompt        string
}

// Service defines the llm interaction contract used by gateway handlers.
type Service interface {
	RegisterNeedInput(sessionID, runID, workerKey, prompt string) (string, error)
	WaitUserInput(runID string, timeout time.Duration) (string, error)
	SubmitUserInput(sessionID, runID, interactionID, input string) (string, error)
	Clear(runID string)
	GetPending(runID string) (PendingView, bool)
	EnsureConversation(runID, conversationID string) string
	ConversationIDByRun(runID string) string
	RunIDByConversation(conversationID string) string
	AppendChatEvent(runID, conversationID string, ev *insightifyv1.ChatEvent)
	SubscribeConversation(conversationID string, fromSeq int64) ([]*insightifyv1.ChatEvent, <-chan *insightifyv1.ChatEvent, func())
}

type Handler struct {
	mu            sync.RWMutex
	byRun         map[string]*pendingInput
	convMu        sync.RWMutex
	conversations map[string]*chatConversation
	runToConv     map[string]string
	convToRun     map[string]string
}

func NewHandler() *Handler {
	return &Handler{
		byRun:         make(map[string]*pendingInput),
		conversations: make(map[string]*chatConversation),
		runToConv:     make(map[string]string),
		convToRun:     make(map[string]string),
	}
}

func (h *Handler) RegisterNeedInput(sessionID, runID, workerKey, prompt string) (string, error) {
	if h == nil {
		return "", fmt.Errorf("llmInteraction handler is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	workerKey = strings.TrimSpace(workerKey)
	if sessionID == "" || runID == "" || workerKey == "" {
		return "", fmt.Errorf("invalid pending input registration")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.byRun[runID]; exists {
		return "", fmt.Errorf("run %s already waiting for user input", runID)
	}

	interactionID := fmt.Sprintf("input-%d", time.Now().UnixNano())
	h.byRun[runID] = &pendingInput{
		InteractionID: interactionID,
		SessionID:     sessionID,
		RunID:         runID,
		WorkerKey:     workerKey,
		Prompt:        strings.TrimSpace(prompt),
		inputCh:       make(chan string, 1),
		done:          make(chan struct{}),
	}
	return interactionID, nil
}

func (h *Handler) WaitUserInput(runID string, timeout time.Duration) (string, error) {
	if h == nil {
		return "", fmt.Errorf("llmInteraction handler is nil")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run_id is required")
	}

	h.mu.RLock()
	pending, ok := h.byRun[runID]
	h.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-pending.done:
		return "", fmt.Errorf("run %s input wait canceled", runID)
	case value := <-pending.inputCh:
		h.Clear(runID)
		return strings.TrimSpace(value), nil
	case <-timer.C:
		h.Clear(runID)
		return "", fmt.Errorf("run %s input wait timed out", runID)
	}
}

func (h *Handler) SubmitUserInput(sessionID, runID, interactionID, input string) (string, error) {
	if h == nil {
		return "", fmt.Errorf("llmInteraction handler is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	interactionID = strings.TrimSpace(interactionID)
	input = strings.TrimSpace(input)
	if sessionID == "" || runID == "" {
		return "", fmt.Errorf("session_id and run_id are required")
	}
	if input == "" {
		return "", fmt.Errorf("input is required")
	}

	h.mu.RLock()
	pending, ok := h.byRun[runID]
	h.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("run %s is not waiting for input", runID)
	}
	if pending.SessionID != sessionID {
		return "", fmt.Errorf("run %s does not belong to session %s", runID, sessionID)
	}
	if interactionID != "" && pending.InteractionID != interactionID {
		return "", fmt.Errorf("interaction_id mismatch for run %s", runID)
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

func (h *Handler) Clear(runID string) {
	if h == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}

	h.mu.Lock()
	pending, ok := h.byRun[runID]
	if ok {
		delete(h.byRun, runID)
	}
	h.mu.Unlock()

	if ok {
		pending.closeDone()
	}
}

func (h *Handler) GetPending(runID string) (PendingView, bool) {
	if h == nil {
		return PendingView{}, false
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return PendingView{}, false
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	pending, ok := h.byRun[runID]
	if !ok || pending == nil {
		return PendingView{}, false
	}
	return PendingView{
		InteractionID: pending.InteractionID,
		SessionID:     pending.SessionID,
		RunID:         pending.RunID,
		WorkerKey:     pending.WorkerKey,
		Prompt:        pending.Prompt,
	}, true
}
