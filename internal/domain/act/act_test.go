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

func TestTransition_Success(t *testing.T) {
	state := &insightifyv1.UiActState{
		ActId:  "act-1",
		Status: insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE,
		Goal:   "test goal",
	}
	out, err := Transition(state, insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.GetStatus() != insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING {
		t.Fatalf("expected PLANNING, got %s", out.GetStatus())
	}
	if out.GetMode() != "planning" {
		t.Fatalf("expected mode=planning, got %s", out.GetMode())
	}
	// Original should be unchanged.
	if state.GetStatus() != insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE {
		t.Fatalf("original state should not be modified")
	}
}

func TestTransition_Denied(t *testing.T) {
	state := &insightifyv1.UiActState{
		ActId:  "act-1",
		Status: insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE,
	}
	_, err := Transition(state, insightifyv1.UiActStatus_UI_ACT_STATUS_DONE)
	if err == nil {
		t.Fatalf("expected error for denied transition")
	}
}

func TestTransition_NilState(t *testing.T) {
	_, err := Transition(nil, insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING)
	if err == nil {
		t.Fatalf("expected error for nil state")
	}
}

func TestAppendTimeline(t *testing.T) {
	state := &insightifyv1.UiActState{
		ActId: "act-1",
		Timeline: []*insightifyv1.UiActTimelineEvent{
			{Id: "evt-1", Kind: "user_input", Summary: "hello"},
		},
	}
	evt := &insightifyv1.UiActTimelineEvent{Id: "evt-2", Kind: "plan", Summary: "thinking..."}
	out := AppendTimeline(state, evt)
	if len(out.GetTimeline()) != 2 {
		t.Fatalf("expected 2 events, got %d", len(out.GetTimeline()))
	}
	if out.GetTimeline()[1].GetId() != "evt-2" {
		t.Fatalf("expected evt-2, got %s", out.GetTimeline()[1].GetId())
	}
	// Original should be unchanged.
	if len(state.GetTimeline()) != 1 {
		t.Fatalf("original state timeline should not be modified")
	}
}

func TestAppendTimeline_Nil(t *testing.T) {
	state := &insightifyv1.UiActState{ActId: "act-1"}
	out := AppendTimeline(state, nil)
	if out != state {
		t.Fatalf("expected same state returned for nil event")
	}
	out = AppendTimeline(nil, &insightifyv1.UiActTimelineEvent{Id: "x"})
	if out != nil {
		t.Fatalf("expected nil returned for nil state")
	}
}

func TestStatusToMode(t *testing.T) {
	cases := []struct {
		status insightifyv1.UiActStatus
		mode   string
	}{
		{insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE, "idle"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING, "planning"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_SUGGESTING, "suggest"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_SEARCHING, "search"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_RUNNING_WORKER, "run_worker"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_NEEDS_USER_ACTION, "needs_user_action"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_DONE, "done"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_FAILED, "failed"},
		{insightifyv1.UiActStatus_UI_ACT_STATUS_UNSPECIFIED, "idle"},
	}
	for _, tc := range cases {
		got := StatusToMode(tc.status)
		if got != tc.mode {
			t.Errorf("StatusToMode(%s) = %q, want %q", tc.status, got, tc.mode)
		}
	}
}
