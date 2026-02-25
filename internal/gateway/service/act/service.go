package act

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// RouteDecision is the outcome of ProcessInput routing.
type RouteDecision struct {
	WorkerKey  string  // selected worker key
	Mode       string  // "suggest" | "search" | "run_worker"
	Confidence float64 // classification confidence 0.0–1.0
	Fallback   bool    // true if autonomous_executor was selected as fallback
	Denied     bool    // true if worker was not in allowed list → needs_user_action
}

// ProcessInput classifies user input, selects a worker, and transitions the act
// node to the appropriate status. It returns the routing decision for the caller
// to use when dispatching the worker.
func (s *Service) ProcessInput(ctx context.Context, runID, nodeID, input string, allowedWorkers []string) (*RouteDecision, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("input is required")
	}

	// 1. Classify input.
	route := actdomain.RouteInput(input)
	decision := &RouteDecision{
		WorkerKey:  route.WorkerKey,
		Mode:       route.Mode,
		Confidence: route.Confidence,
	}

	// 2. Fallback check.
	if actdomain.ShouldFallback(route.Confidence) {
		decision.WorkerKey = "autonomous_executor"
		decision.Fallback = true
	}

	// 3. Allowed workers check.
	if !actdomain.IsWorkerAllowed(decision.WorkerKey, allowedWorkers) {
		decision.Denied = true
		// Transition to needs_user_action.
		if err := s.TransitionAct(ctx, runID, nodeID, insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION); err != nil {
			return nil, fmt.Errorf("failed to transition act to needs_user_action: %w", err)
		}
		_ = s.AppendTimeline(ctx, runID, nodeID, &insightifyv1.UiActTimelineEvent{
			Id:      fmt.Sprintf("route-%d", timeNowUnixMs()),
			Kind:    "system_note",
			Summary: fmt.Sprintf("Worker %q is not in allowed list. Please choose from: %s", decision.WorkerKey, strings.Join(allowedWorkers, ", ")),
		})
		return decision, nil
	}

	// 4. Transition act to the appropriate status.
	targetStatus := modeToStatus(decision.Mode)
	if err := s.TransitionAct(ctx, runID, nodeID, targetStatus); err != nil {
		return nil, fmt.Errorf("failed to transition act to %s: %w", targetStatus, err)
	}

	// 5. Append routing timeline event.
	summary := fmt.Sprintf("Routing: mode=%s, worker=%s, confidence=%.2f", decision.Mode, decision.WorkerKey, decision.Confidence)
	if decision.Fallback {
		summary += " (fallback)"
	}
	_ = s.AppendTimeline(ctx, runID, nodeID, &insightifyv1.UiActTimelineEvent{
		Id:      fmt.Sprintf("route-%d", timeNowUnixMs()),
		Kind:    "plan",
		Summary: summary,
	})

	return decision, nil
}

// modeToStatus maps a route mode string to the corresponding UiActStatus.
func modeToStatus(mode string) insightifyv1.UiActStatus {
	switch mode {
	case "suggest":
		return insightifyv1.UiActStatus_UI_ACT_STATUS_SUGGESTING
	case "search":
		return insightifyv1.UiActStatus_UI_ACT_STATUS_SEARCHING
	case "run_worker":
		return insightifyv1.UiActStatus_UI_ACT_STATUS_RUNNING_WORKER
	default:
		return insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING
	}
}

// timeNowUnixMs returns the current time in milliseconds. Extracted for testability.
func timeNowUnixMs() int64 {
	return time.Now().UnixMilli()
}
