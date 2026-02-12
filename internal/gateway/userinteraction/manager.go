package userinteraction

import (
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	llminteraction "insightify/internal/llmInteraction"
)

type Manager struct {
	interaction llminteraction.Service

	mu              sync.RWMutex
	activeRunByProj map[string]string
	projectByRun    map[string]string
}

func New() *Manager {
	return &Manager{
		interaction:     llminteraction.NewHandler(),
		activeRunByProj: make(map[string]string),
		projectByRun:    make(map[string]string),
	}
}

func (m *Manager) MarkRunStarted(projectID, runID string) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	if m == nil || projectID == "" || runID == "" {
		return
	}
	m.mu.Lock()
	m.activeRunByProj[projectID] = runID
	m.projectByRun[runID] = projectID
	m.mu.Unlock()
}

func (m *Manager) MarkRunFinished(projectID, runID string) {
	projectID = strings.TrimSpace(projectID)
	runID = strings.TrimSpace(runID)
	if m == nil || projectID == "" || runID == "" {
		return
	}
	m.mu.Lock()
	if cur := strings.TrimSpace(m.activeRunByProj[projectID]); cur == runID {
		delete(m.activeRunByProj, projectID)
	}
	delete(m.projectByRun, runID)
	m.mu.Unlock()
}

func (m *Manager) ActiveRunID(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	if m == nil || projectID == "" {
		return ""
	}
	m.mu.RLock()
	runID := strings.TrimSpace(m.activeRunByProj[projectID])
	m.mu.RUnlock()
	return runID
}

func (m *Manager) ProjectIDByRun(runID string) string {
	runID = strings.TrimSpace(runID)
	if m == nil || runID == "" {
		return ""
	}
	m.mu.RLock()
	projectID := strings.TrimSpace(m.projectByRun[runID])
	m.mu.RUnlock()
	return projectID
}

func (m *Manager) EnsureConversation(runID, conversationID string) string {
	if m == nil {
		return ""
	}
	return m.interaction.EnsureConversation(runID, conversationID)
}

func (m *Manager) ConversationIDByRun(runID string) string {
	if m == nil {
		return ""
	}
	return m.interaction.ConversationIDByRun(runID)
}

func (m *Manager) RunIDByConversation(conversationID string) string {
	if m == nil {
		return ""
	}
	return m.interaction.RunIDByConversation(conversationID)
}

func (m *Manager) AppendChatEvent(runID, conversationID string, ev *insightifyv1.ChatEvent) {
	if m == nil {
		return
	}
	m.interaction.AppendChatEvent(runID, conversationID, ev)
}

func (m *Manager) SubscribeConversation(conversationID string, fromSeq int64) ([]*insightifyv1.ChatEvent, <-chan *insightifyv1.ChatEvent, func()) {
	if m == nil {
		return nil, nil, nil
	}
	return m.interaction.SubscribeConversation(conversationID, fromSeq)
}

func (m *Manager) RegisterNeedInput(projectID, runID, workerKey, prompt string) (string, error) {
	if m == nil {
		return "", nil
	}
	return m.interaction.RegisterNeedInput(projectID, runID, workerKey, prompt)
}

func (m *Manager) WaitUserInput(runID string, timeout time.Duration) (string, error) {
	if m == nil {
		return "", nil
	}
	return m.interaction.WaitUserInput(runID, timeout)
}

func (m *Manager) SubmitUserInput(projectID, runID, interactionID, input string) (string, error) {
	if m == nil {
		return "", nil
	}
	return m.interaction.SubmitUserInput(projectID, runID, interactionID, input)
}

func (m *Manager) Clear(runID string) {
	if m == nil {
		return
	}
	m.interaction.Clear(runID)
}

func (m *Manager) GetPending(runID string) (llminteraction.PendingView, bool) {
	if m == nil {
		return llminteraction.PendingView{}, false
	}
	return m.interaction.GetPending(runID)
}
