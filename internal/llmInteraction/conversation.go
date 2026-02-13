package llminteraction

import (
	"strings"
	"sync"
)

// chatConversation tracks a run↔conversation binding.
// Event storage has been removed; only the ID mapping is retained.
type chatConversation struct {
	mu sync.RWMutex
}

func defaultConversationID(runID string) string {
	return strings.TrimSpace(runID)
}

func (h *Handler) EnsureConversation(runID, conversationID string) string {
	if h == nil {
		return ""
	}
	runID = strings.TrimSpace(runID)
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		conversationID = defaultConversationID(runID)
	}
	if conversationID == "" {
		return ""
	}

	h.convMu.Lock()
	defer h.convMu.Unlock()

	// Keep existing binding if already present.
	if runID != "" {
		if existingID := strings.TrimSpace(h.runToConv[runID]); existingID != "" && existingID != conversationID {
			// Rebind to new conversation id.
		}
	}

	if h.conversations[conversationID] == nil {
		h.conversations[conversationID] = &chatConversation{}
	}
	if runID != "" {
		h.runToConv[runID] = conversationID
		h.convToRun[conversationID] = runID
	}
	return conversationID
}

func (h *Handler) ConversationIDByRun(runID string) string {
	if h == nil {
		return ""
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	h.convMu.RLock()
	convID := h.runToConv[runID]
	h.convMu.RUnlock()
	if strings.TrimSpace(convID) != "" {
		return convID
	}
	return defaultConversationID(runID)
}

func (h *Handler) RunIDByConversation(conversationID string) string {
	if h == nil {
		return ""
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ""
	}
	h.convMu.RLock()
	runID := h.convToRun[conversationID]
	h.convMu.RUnlock()
	return strings.TrimSpace(runID)
}
