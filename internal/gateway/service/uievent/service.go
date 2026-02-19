package uievent

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	uirepo "insightify/internal/gateway/repository/ui"
)

type EventType string

const (
	EventUserAccepted  EventType = "user_accepted"
	EventAssistantOut  EventType = "assistant_out"
	EventAssistantDone EventType = "assistant_done"
	EventWaiting       EventType = "waiting"
)

type Event struct {
	RunID         string
	InteractionID string
	Type          EventType
	Content       string
	Waiting       bool
}

type Service struct {
	store uirepo.Store
}

func New(store uirepo.Store) *Service {
	return &Service{store: store}
}

func (s *Service) OnUserAccepted(ctx context.Context, runID, interactionID, input string) error {
	return s.Handle(ctx, Event{
		RunID:         runID,
		InteractionID: interactionID,
		Type:          EventUserAccepted,
		Content:       input,
	})
}

func (s *Service) OnAssistantOutput(ctx context.Context, runID, interactionID, message string) error {
	if err := s.Handle(ctx, Event{
		RunID:         runID,
		InteractionID: interactionID,
		Type:          EventAssistantOut,
		Content:       message,
	}); err != nil {
		return err
	}
	return s.Handle(ctx, Event{
		RunID:         runID,
		InteractionID: interactionID,
		Type:          EventAssistantDone,
	})
}

func (s *Service) OnWaiting(ctx context.Context, runID, interactionID string, waiting bool) error {
	return s.Handle(ctx, Event{
		RunID:         runID,
		InteractionID: interactionID,
		Type:          EventWaiting,
		Waiting:       waiting,
	})
}

func (s *Service) Handle(ctx context.Context, ev Event) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("ui event service is not available")
	}
	runID := strings.TrimSpace(ev.RunID)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}

	doc, err := s.store.GetDocument(ctx, runID)
	if err != nil {
		return err
	}
	node := findOrCreateChatNode(doc, runID)
	if node == nil {
		return fmt.Errorf("failed to resolve chat node for run %s", runID)
	}
	applyEventToNode(node, ev)

	_, _, err = s.store.ApplyOps(ctx, runID, 0, []*insightifyv1.UiOp{
		{
			Action: &insightifyv1.UiOp_UpsertNode{
				UpsertNode: &insightifyv1.UiUpsertNode{Node: node},
			},
		},
	})
	return err
}

func findOrCreateChatNode(doc *insightifyv1.UiDocument, runID string) *insightifyv1.UiNode {
	if doc != nil {
		for _, node := range doc.GetNodes() {
			if node == nil {
				continue
			}
			if node.GetType() == insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT {
				if node.Meta == nil {
					node.Meta = &insightifyv1.UiNodeMeta{Title: "LLM Chat"}
				}
				if node.LlmChat == nil {
					node.LlmChat = &insightifyv1.UiLlmChatState{}
				}
				return node
			}
		}
	}
	return &insightifyv1.UiNode{
		Id:   "llm-chat-" + sanitizeID(runID),
		Type: insightifyv1.UiNodeType_UI_NODE_TYPE_LLM_CHAT,
		Meta: &insightifyv1.UiNodeMeta{
			Title: "LLM Chat",
		},
		LlmChat: &insightifyv1.UiLlmChatState{
			Model:        "Low",
			IsResponding: false,
			SendLocked:   false,
			SendLockHint: "",
			Messages:     nil,
		},
	}
}

func applyEventToNode(node *insightifyv1.UiNode, ev Event) {
	if node == nil {
		return
	}
	if node.LlmChat == nil {
		node.LlmChat = &insightifyv1.UiLlmChatState{}
	}
	chat := node.LlmChat
	if strings.TrimSpace(chat.GetModel()) == "" {
		chat.Model = "Low"
	}
	interactionID := strings.TrimSpace(ev.InteractionID)
	if interactionID == "" {
		interactionID = fmt.Sprintf("interaction-%d", time.Now().UnixNano())
	}

	switch ev.Type {
	case EventUserAccepted:
		content := strings.TrimSpace(ev.Content)
		if content != "" {
			chat.Messages = append(chat.Messages, &insightifyv1.UiChatMessage{
				Id:      fmt.Sprintf("%s-user-%d", sanitizeID(interactionID), len(chat.Messages)+1),
				Role:    insightifyv1.UiChatMessage_ROLE_USER,
				Content: content,
			})
		}
		chat.IsResponding = true
		chat.SendLocked = false
		chat.SendLockHint = ""
	case EventAssistantOut:
		content := strings.TrimSpace(ev.Content)
		if content != "" {
			chat.Messages = append(chat.Messages, &insightifyv1.UiChatMessage{
				Id:      fmt.Sprintf("%s-assistant-%d", sanitizeID(interactionID), len(chat.Messages)+1),
				Role:    insightifyv1.UiChatMessage_ROLE_ASSISTANT,
				Content: content,
			})
		}
		chat.IsResponding = true
		chat.SendLocked = false
		chat.SendLockHint = ""
	case EventAssistantDone:
		chat.IsResponding = false
		chat.SendLocked = false
		chat.SendLockHint = ""
	case EventWaiting:
		chat.IsResponding = false
		chat.SendLocked = false
		if ev.Waiting {
			chat.SendLockHint = "Waiting for user input"
		} else {
			chat.SendLockHint = ""
		}
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
