package ui

import "strings"

// NeedUserInput builds an LLM chat node representing "waiting for user input".
func NeedUserInput(nodeID, title, model, prompt string, history []ChatMessage) (Node, bool) {
	prompt = strings.TrimSpace(prompt)
	messages := append([]ChatMessage(nil), history...)
	if prompt != "" {
		messages = append(messages, ChatMessage{
			ID:      "assistant-need-input",
			Role:    RoleAssistant,
			Content: prompt,
		})
	}
	return BuildChatNode(nodeID, title, model, messages, false, true, prompt)
}

// Followup builds an LLM chat node representing assistant follow-up output.
func Followup(nodeID, title, model, assistantMessage string, needMoreInput bool, followupQuestion string, history []ChatMessage) (Node, bool) {
	msg := strings.TrimSpace(assistantMessage)
	followupQuestion = strings.TrimSpace(followupQuestion)
	messages := append([]ChatMessage(nil), history...)
	if msg != "" {
		messages = append(messages, ChatMessage{
			ID:      "assistant-followup",
			Role:    RoleAssistant,
			Content: msg,
		})
	}
	if needMoreInput {
		return BuildChatNode(nodeID, title, model, messages, false, true, followupQuestion)
	}
	return BuildChatNode(nodeID, title, model, messages, false, false, "")
}
