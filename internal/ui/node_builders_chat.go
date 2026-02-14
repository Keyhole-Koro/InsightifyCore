package ui

import (
	"fmt"
	"strings"
)

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
