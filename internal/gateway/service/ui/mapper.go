package ui

import (
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	workerv1 "insightify/gen/go/worker/v1"
)

func (s *Service) UpsertFromClientView(runID, workerID string, view *workerv1.ClientView) *insightifyv1.UiNode {
	node := nodeFromClientView(workerID, view)
	if node == nil {
		return nil
	}
	s.Set(runID, node)
	return node
}

func nodeFromClientView(workerID string, view *workerv1.ClientView) *insightifyv1.UiNode {
	if view == nil {
		return nil
	}
	content := strings.TrimSpace(view.GetLlmResponse())
	if content == "" {
		return nil
	}
	return &insightifyv1.UiNode{
		Id:   fmt.Sprintf("%s-node", sanitizeID(workerID)),
		Type: insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT,
		Meta: &insightifyv1.UiNodeMeta{
			Title: strings.TrimSpace(workerID),
		},
		LlmChat: &insightifyv1.UiLlmChatState{
			Model:        "Low",
			IsResponding: false,
			SendLocked:   false,
			Messages: []*insightifyv1.UiChatMessage{
				{
					Id:      fmt.Sprintf("%s-assistant-1", sanitizeID(workerID)),
					Role:    insightifyv1.UiChatMessage_ROLE_ASSISTANT,
					Content: content,
				},
			},
		},
	}
}

func sanitizeID(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "unknown"
	}
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "/", "_")
	return v
}
