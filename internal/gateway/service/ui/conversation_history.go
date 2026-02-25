package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

type conversationArtifact struct {
	RunID    string                `json:"run_id"`
	Messages []conversationMessage `json:"messages"`
}

type conversationMessage struct {
	Seq           int    `json:"seq"`
	Role          string `json:"role"`
	Content       string `json:"content"`
	InteractionID string `json:"interaction_id,omitempty"`
}

func (s *Service) withConversationHistory(ctx context.Context, runID string, doc *insightifyv1.UiDocument) *insightifyv1.UiDocument {
	if s == nil || s.artifact == nil || doc == nil {
		return doc
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return doc
	}
	out := cloneDocument(doc)
	basePath := strings.TrimSpace(s.conversationArtifactPath)
	if basePath == "" {
		return out
	}
	for _, node := range out.GetNodes() {
		if node == nil || node.GetType() != insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT {
			continue
		}
		nodeID := strings.TrimSpace(node.GetId())
		if nodeID == "" {
			continue
		}
		raw, err := s.artifact.Get(ctx, runID, nodeID+"/"+basePath)
		if err != nil || len(raw) == 0 {
			continue
		}
		var conv conversationArtifact
		if err := json.Unmarshal(raw, &conv); err != nil {
			continue
		}
		msgs := mapConversationMessages(conv.Messages)
		if len(msgs) == 0 {
			continue
		}
		if node.LlmChat == nil {
			node.LlmChat = &insightifyv1.UiLlmChatState{}
		}
		node.LlmChat.Messages = msgs
		node.LlmChat.IsResponding = false
		node.LlmChat.SendLocked = false
		if strings.TrimSpace(node.LlmChat.Model) == "" {
			node.LlmChat.Model = "Low"
		}
	}
	return out
}

func mapConversationMessages(messages []conversationMessage) []*insightifyv1.UiChatMessage {
	msgs := make([]*insightifyv1.UiChatMessage, 0, len(messages))
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		role := insightifyv1.UiChatMessage_ROLE_UNSPECIFIED
		switch strings.ToLower(strings.TrimSpace(m.Role)) {
		case "user":
			role = insightifyv1.UiChatMessage_ROLE_USER
		case "assistant":
			role = insightifyv1.UiChatMessage_ROLE_ASSISTANT
		default:
			continue
		}
		id := strings.TrimSpace(m.InteractionID)
		if id == "" {
			id = fmt.Sprintf("chat-%d", len(msgs)+1)
		}
		id = fmt.Sprintf("%s-%d", sanitizeID(id), len(msgs)+1)
		msgs = append(msgs, &insightifyv1.UiChatMessage{
			Id:      id,
			Role:    role,
			Content: content,
		})
	}
	return msgs
}

func cloneDocument(in *insightifyv1.UiDocument) *insightifyv1.UiDocument {
	if in == nil {
		return nil
	}
	out := &insightifyv1.UiDocument{
		RunId:   in.GetRunId(),
		Version: in.GetVersion(),
	}
	nodes := in.GetNodes()
	if len(nodes) == 0 {
		return out
	}
	out.Nodes = append([]*insightifyv1.UiNode(nil), nodes...)
	return out
}
