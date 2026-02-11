package ui

import (
	"fmt"
	"strings"
)

func BuildChatNode(id, title, model string, messages []ChatMessage, isResponding, sendLocked bool, sendLockHint string) (Node, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Node{}, false
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "LLM Chat"
	}
	model = strings.TrimSpace(model)
	sendLockHint = strings.TrimSpace(sendLockHint)
	filtered := make([]ChatMessage, 0, len(messages))
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		role := m.Role
		if role != RoleUser && role != RoleAssistant {
			continue
		}
		messageID := strings.TrimSpace(m.ID)
		if messageID == "" {
			messageID = fmt.Sprintf("%s-%s-%d", id, role, len(filtered)+1)
		}
		filtered = append(filtered, ChatMessage{
			ID:      messageID,
			Role:    role,
			Content: content,
		})
	}

	return Node{
		ID:   id,
		Type: NodeTypeLLMChat,
		Meta: Meta{
			Title: title,
		},
		LLMChat: &LLMChatState{
			Model:        model,
			IsResponding: isResponding,
			SendLocked:   sendLocked,
			SendLockHint: sendLockHint,
			Messages:     filtered,
		},
	}, true
}

func BuildLLMChatNode(runID, workerKey, text string, seq int64, isResponding bool, sendLocked bool, sendLockHint string) (Node, bool) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Node{}, false
	}

	workerKey = strings.TrimSpace(workerKey)
	if workerKey == "" {
		parts := strings.SplitN(runID, "-", 2)
		if len(parts) > 0 {
			workerKey = strings.TrimSpace(parts[0])
		}
	}

	title := "LLM Chat"
	if workerKey != "" {
		title = workerKey
	}

	message := strings.TrimSpace(text)
	messages := make([]ChatMessage, 0, 1)
	if message != "" {
		messages = append(messages, ChatMessage{
			ID:      fmt.Sprintf("%s-assistant-%d", runID, seq),
			Role:    RoleAssistant,
			Content: message,
		})
	}

	return Node{
		ID:   runID,
		Type: NodeTypeLLMChat,
		Meta: Meta{
			Title: title,
		},
		LLMChat: &LLMChatState{
			IsResponding: isResponding,
			SendLocked:   sendLocked,
			SendLockHint: strings.TrimSpace(sendLockHint),
			Messages:     messages,
		},
	}, true
}
