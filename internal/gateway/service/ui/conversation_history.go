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
	path := strings.TrimSpace(s.conversationArtifactPath)
	if path == "" {
		return doc
	}
	raw, err := s.artifact.Get(ctx, runID, path)
	if err != nil || len(raw) == 0 {
		return doc
	}
	var conv conversationArtifact
	if err := json.Unmarshal(raw, &conv); err != nil {
		return doc
	}
	if len(conv.Messages) == 0 {
		return doc
	}

	msgs := make([]*insightifyv1.UiChatMessage, 0, len(conv.Messages))
	for _, m := range conv.Messages {
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
	if len(msgs) == 0 {
		return doc
	}

	out := cloneDocument(doc)
	node := findOrCreateChatNode(out, runID)
	node.LlmChat.Messages = msgs
	node.LlmChat.IsResponding = false
	node.LlmChat.SendLocked = false
	if strings.TrimSpace(node.LlmChat.Model) == "" {
		node.LlmChat.Model = "Low"
	}
	return out
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

func findOrCreateChatNode(doc *insightifyv1.UiDocument, runID string) *insightifyv1.UiNode {
	if doc == nil {
		return nil
	}
	for _, n := range doc.GetNodes() {
		if n == nil {
			continue
		}
		if n.GetType() == insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT {
			if n.LlmChat == nil {
				n.LlmChat = &insightifyv1.UiLlmChatState{}
			}
			return n
		}
	}
	node := &insightifyv1.UiNode{
		Id:   "llm-chat-" + sanitizeID(runID),
		Type: insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT,
		Meta: &insightifyv1.UiNodeMeta{
			Title: "LLM Chat",
		},
		LlmChat: &insightifyv1.UiLlmChatState{},
	}
	doc.Nodes = append(doc.Nodes, node)
	return node
}
