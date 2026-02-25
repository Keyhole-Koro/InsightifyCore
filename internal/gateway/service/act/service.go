package act

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	insightifyv1 "insightify/gen/go/insightify/v1"
	actdomain "insightify/internal/domain/act"
	uirepo "insightify/internal/gateway/repository/ui"
)

// UIDocReader provides read access to the UI document store.
type UIDocReader interface {
	GetDocument(ctx context.Context, runID string) (*insightifyv1.UiDocument, error)
}

// UIDocWriter provides write access to the UI document store via ops.
type UIDocWriter interface {
	ApplyOps(ctx context.Context, runID string, baseVersion int32, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, int32, error)
}

// Service orchestrates act state transitions, timeline events, and future
// worker dispatch. It reads/writes act node state through the UI document store.
type Service struct {
	store uirepo.Store
}

// New creates a new act orchestrator service.
func New(store uirepo.Store) *Service {
	return &Service{store: store}
}

// TransitionAct validates and applies a status transition to the act node
// identified by (runID, nodeID). The act's mode string is updated automatically.
func (s *Service) TransitionAct(ctx context.Context, runID, nodeID string, to insightifyv1.UiActStatus) error {
	return s.updateActNode(ctx, runID, nodeID, func(state *insightifyv1.UiActState) (*insightifyv1.UiActState, error) {
		return actdomain.Transition(state, to)
	})
}

// AppendTimeline adds a timeline event to the act node.
func (s *Service) AppendTimeline(ctx context.Context, runID, nodeID string, evt *insightifyv1.UiActTimelineEvent) error {
	if evt == nil {
		return nil
	}
	return s.updateActNode(ctx, runID, nodeID, func(state *insightifyv1.UiActState) (*insightifyv1.UiActState, error) {
		return actdomain.AppendTimeline(state, evt), nil
	})
}

// SetGoal sets the goal on an act node (typically on first user input).
func (s *Service) SetGoal(ctx context.Context, runID, nodeID, goal string) error {
	return s.updateActNode(ctx, runID, nodeID, func(state *insightifyv1.UiActState) (*insightifyv1.UiActState, error) {
		out := proto.Clone(state).(*insightifyv1.UiActState)
		out.Goal = strings.TrimSpace(goal)
		return out, nil
	})
}

// updateActNode reads the current act state from the UI document, applies the
// mutator function, and writes the updated node back via ApplyOps.
func (s *Service) updateActNode(ctx context.Context, runID, nodeID string, mutate func(*insightifyv1.UiActState) (*insightifyv1.UiActState, error)) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("act service is not available")
	}
	runID = strings.TrimSpace(runID)
	nodeID = strings.TrimSpace(nodeID)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if nodeID == "" {
		return fmt.Errorf("node_id is required")
	}

	// Read current document to find the act node.
	doc, err := s.store.GetDocument(ctx, runID)
	if err != nil {
		return fmt.Errorf("failed to read ui document for run %s: %w", runID, err)
	}

	var targetNode *insightifyv1.UiNode
	for _, n := range doc.GetNodes() {
		if strings.TrimSpace(n.GetId()) == nodeID {
			targetNode = n
			break
		}
	}
	if targetNode == nil {
		return fmt.Errorf("node %s not found in run %s", nodeID, runID)
	}
	actState := targetNode.GetAct()
	if actState == nil {
		return fmt.Errorf("node %s has no act state", nodeID)
	}

	// Apply mutation.
	updated, err := mutate(actState)
	if err != nil {
		return err
	}
	if updated == nil {
		return nil // no-op
	}

	// Write back via UpsertNode op.
	updatedNode := proto.Clone(targetNode).(*insightifyv1.UiNode)
	updatedNode.Act = updated
	_, _, err = s.store.ApplyOps(ctx, runID, 0, []*insightifyv1.UiOp{
		{
			Action: &insightifyv1.UiOp_UpsertNode{
				UpsertNode: &insightifyv1.UiUpsertNode{Node: updatedNode},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to apply act update for node %s in run %s: %w", nodeID, runID, err)
	}
	return nil
}
