package act

import insightifyv1 "insightify/gen/go/insightify/v1"

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
