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
	NodeID        string
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

func (s *Service) OnUserAccepted(ctx context.Context, runID, nodeID, interactionID, input string) error {
	return s.Handle(ctx, Event{
		RunID:         runID,
		NodeID:        nodeID,
		InteractionID: interactionID,
		Type:          EventUserAccepted,
		Content:       input,
	})
}

func (s *Service) OnAssistantOutput(ctx context.Context, runID, nodeID, interactionID, message string) error {
	if err := s.Handle(ctx, Event{
		RunID:         runID,
		NodeID:        nodeID,
		InteractionID: interactionID,
		Type:          EventAssistantOut,
		Content:       message,
	}); err != nil {
		return err
	}
	return s.Handle(ctx, Event{
		RunID:         runID,
		NodeID:        nodeID,
		InteractionID: interactionID,
		Type:          EventAssistantDone,
	})
}

func (s *Service) OnWaiting(ctx context.Context, runID, nodeID, interactionID string, waiting bool) error {
	return s.Handle(ctx, Event{
		RunID:         runID,
		NodeID:        nodeID,
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
	nodeID := strings.TrimSpace(ev.NodeID)
	if runID == "" || nodeID == "" {
		return fmt.Errorf("run_id and node_id are required")
	}

	doc, err := s.store.GetDocument(ctx, runID)
	if err != nil {
		return err
	}
	node := findActNodeByID(doc, nodeID)
	if node == nil {
		return fmt.Errorf("act node not found: run_id=%s node_id=%s", runID, nodeID)
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

func findActNodeByID(doc *insightifyv1.UiDocument, nodeID string) *insightifyv1.UiNode {
	if doc != nil {
		for _, node := range doc.GetNodes() {
			if node == nil {
				continue
			}
			if strings.TrimSpace(node.GetId()) == nodeID &&
				node.GetType() == insightifyv1.UiNodeType_UI_NODE_TYPE_ACT {
				if node.Meta == nil {
					node.Meta = &insightifyv1.UiNodeMeta{Title: "Act"}
				}
				if node.Act == nil {
					node.Act = &insightifyv1.UiActState{
						ActId: node.GetId(),
					}
				}
				return node
			}
		}
	}
	return nil
}

func applyEventToNode(node *insightifyv1.UiNode, ev Event) {
	if node == nil {
		return
	}
	if node.Act == nil {
		node.Act = &insightifyv1.UiActState{
			ActId: node.GetId(),
		}
	}
	act := node.Act
	if strings.TrimSpace(act.GetActId()) == "" {
		act.ActId = node.GetId()
	}
	interactionID := strings.TrimSpace(ev.InteractionID)
	if interactionID == "" {
		interactionID = fmt.Sprintf("interaction-%d", time.Now().UnixNano())
	}

	switch ev.Type {
	case EventUserAccepted:
		content := strings.TrimSpace(ev.Content)
		if content != "" {
			act.Timeline = append(act.GetTimeline(), &insightifyv1.UiActTimelineEvent{
				Id:              fmt.Sprintf("%s-user-%d", sanitizeID(interactionID), len(act.GetTimeline())+1),
				CreatedAtUnixMs: time.Now().UnixMilli(),
				Kind:            "user_input",
				Summary:         content,
				Detail:          interactionID,
			})
		}
		act.Status = insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING
		act.Mode = "planning"
	case EventAssistantOut:
		content := strings.TrimSpace(ev.Content)
		if content != "" {
			act.Timeline = append(act.GetTimeline(), &insightifyv1.UiActTimelineEvent{
				Id:              fmt.Sprintf("%s-assistant-%d", sanitizeID(interactionID), len(act.GetTimeline())+1),
				CreatedAtUnixMs: time.Now().UnixMilli(),
				Kind:            "worker_output",
				Summary:         content,
				Detail:          interactionID,
			})
		}
		act.Status = insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION
		act.Mode = "needs_user_action"
	case EventAssistantDone:
		act.Status = insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION
		act.Mode = "needs_user_action"
	case EventWaiting:
		if ev.Waiting {
			act.Mode = "needs_user_action"
			act.Status = insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION
		} else {
			if strings.TrimSpace(act.GetMode()) == "" {
				act.Mode = "planning"
			}
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
