package act

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

// CanTransition returns true when an act status transition is allowed.
func CanTransition(from, to insightifyv1.UiActStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case insightifyv1.UiActStatus_UI_ACT_STATUS_UNSPECIFIED:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING
	case insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING
	case insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_SUGGESTING ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_SEARCHING ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_RUNNING_WORKER ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED
	case insightifyv1.UiActStatus_UI_ACT_STATUS_SUGGESTING, insightifyv1.UiActStatus_UI_ACT_STATUS_SEARCHING:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED
	case insightifyv1.UiActStatus_UI_ACT_STATUS_RUNNING_WORKER:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_DONE ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED
	case insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING ||
			to == insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED
	case insightifyv1.UiActStatus_UI_ACT_STATUS_DONE:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING
	case insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED:
		return to == insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING
	default:
		return false
	}
}

// IsTerminalStatus reports whether status is terminal for the current step.
func IsTerminalStatus(status insightifyv1.UiActStatus) bool {
	return status == insightifyv1.UiActStatus_UI_ACT_STATUS_DONE ||
		status == insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED
}

// Transition validates the status change and returns a new UiActState with the
// updated status. Returns an error when the transition is not allowed.
func Transition(state *insightifyv1.UiActState, to insightifyv1.UiActStatus) (*insightifyv1.UiActState, error) {
	if state == nil {
		return nil, fmt.Errorf("act state is nil")
	}
	from := state.GetStatus()
	if !CanTransition(from, to) {
		return nil, fmt.Errorf("act transition %s -> %s is not allowed", from, to)
	}
	out := proto.Clone(state).(*insightifyv1.UiActState)
	out.Status = to
	// Derive mode string from status for lightweight UI consumption.
	out.Mode = StatusToMode(to)
	return out, nil
}

// AppendTimeline returns a copy of state with the given event appended to the
// timeline slice. The original state is not modified.
func AppendTimeline(state *insightifyv1.UiActState, evt *insightifyv1.UiActTimelineEvent) *insightifyv1.UiActState {
	if state == nil || evt == nil {
		return state
	}
	out := proto.Clone(state).(*insightifyv1.UiActState)
	out.Timeline = append(out.GetTimeline(), evt)
	return out
}

// StatusToMode returns a human-readable mode string for the given act status.
func StatusToMode(s insightifyv1.UiActStatus) string {
	switch s {
	case insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE:
		return "idle"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING:
		return "planning"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_SUGGESTING:
		return "suggest"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_SEARCHING:
		return "search"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_RUNNING_WORKER:
		return "run_worker"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION:
		return "needs_user_action"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_DONE:
		return "done"
	case insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED:
		return "failed"
	default:
		return "idle"
	}
}
