package act

import (
	"testing"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

func TestCanTransition(t *testing.T) {
	if !CanTransition(insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE, insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING) {
		t.Fatalf("expected IDLE -> PLANNING transition to be allowed")
	}
	if CanTransition(insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE, insightifyv1.UiActStatus_UI_ACT_STATUS_DONE) {
		t.Fatalf("expected IDLE -> DONE transition to be denied")
	}
}

func TestIsNodeCreateActorAllowed(t *testing.T) {
	if !IsNodeCreateActorAllowed("act") {
		t.Fatalf("expected act actor to be allowed")
	}
	if IsNodeCreateActorAllowed("user") {
		t.Fatalf("expected user actor to be denied")
	}
}
