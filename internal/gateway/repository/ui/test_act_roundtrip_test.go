package ui

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

// TestActPayloadRoundTrip verifies that a UiNode with a full act payload
// survives the marshalNodes → unmarshalNodes cycle used by the Postgres store.
// This ensures status, mode, goal, timeline, and pending_actions are preserved.
func TestActPayloadRoundTrip(t *testing.T) {
	original := map[string]*insightifyv1.UiNode{
		"act-1": {
			Id:   "act-1",
			Type: insightifyv1.UiNodeType_UI_NODE_TYPE_ACT,
			Meta: &insightifyv1.UiNodeMeta{
				Title:       "Test Act",
				Description: "A test act node",
			},
			Act: &insightifyv1.UiActState{
				ActId:          "act-1",
				Status:         insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING,
				Mode:           "planning",
				Goal:           "implement feature X",
				SelectedWorker: "bootstrap",
				PendingActions: []*insightifyv1.UiActPendingAction{
					{Id: "pa-1", Label: "Confirm", Description: "Confirm the plan"},
					{Id: "pa-2", Label: "Abort", Description: "Abort execution"},
				},
				Timeline: []*insightifyv1.UiActTimelineEvent{
					{Id: "evt-1", CreatedAtUnixMs: 1000, Kind: "user_input", Summary: "implement feature X"},
					{Id: "evt-2", CreatedAtUnixMs: 2000, Kind: "plan", Summary: "Routing: mode=run_worker", WorkerKey: "bootstrap"},
				},
			},
		},
	}

	// Marshal → JSON bytes (same as store)
	marshaled, err := marshalNodes(original)
	if err != nil {
		t.Fatalf("marshalNodes failed: %v", err)
	}

	// Simulate Ent storage round-trip: bytes → map[string]any → bytes
	var intermediate map[string]any
	if err := json.Unmarshal(marshaled, &intermediate); err != nil {
		t.Fatalf("json unmarshal to map[string]any failed: %v", err)
	}

	// Unmarshal back (same as store read path)
	restored, err := unmarshalNodes(intermediate)
	if err != nil {
		t.Fatalf("unmarshalNodes failed: %v", err)
	}

	// Verify node exists
	node, ok := restored["act-1"]
	if !ok {
		t.Fatal("act-1 not found in restored nodes")
	}

	// Verify basic fields
	if node.GetId() != "act-1" {
		t.Errorf("id = %q, want act-1", node.GetId())
	}
	if node.GetType() != insightifyv1.UiNodeType_UI_NODE_TYPE_ACT {
		t.Errorf("type = %v, want ACT", node.GetType())
	}

	// Verify act state
	act := node.GetAct()
	if act == nil {
		t.Fatal("act state is nil after round-trip")
	}
	if act.GetActId() != "act-1" {
		t.Errorf("act_id = %q, want act-1", act.GetActId())
	}
	if act.GetStatus() != insightifyv1.UiActStatus_UI_ACT_STATUS_PLANNING {
		t.Errorf("status = %v, want PLANNING", act.GetStatus())
	}
	if act.GetMode() != "planning" {
		t.Errorf("mode = %q, want planning", act.GetMode())
	}
	if act.GetGoal() != "implement feature X" {
		t.Errorf("goal = %q, want 'implement feature X'", act.GetGoal())
	}
	if act.GetSelectedWorker() != "bootstrap" {
		t.Errorf("selected_worker = %q, want bootstrap", act.GetSelectedWorker())
	}

	// Verify pending_actions
	if len(act.GetPendingActions()) != 2 {
		t.Fatalf("pending_actions length = %d, want 2", len(act.GetPendingActions()))
	}
	if act.GetPendingActions()[0].GetId() != "pa-1" {
		t.Errorf("pending_actions[0].id = %q, want pa-1", act.GetPendingActions()[0].GetId())
	}
	if act.GetPendingActions()[1].GetLabel() != "Abort" {
		t.Errorf("pending_actions[1].label = %q, want Abort", act.GetPendingActions()[1].GetLabel())
	}

	// Verify timeline
	if len(act.GetTimeline()) != 2 {
		t.Fatalf("timeline length = %d, want 2", len(act.GetTimeline()))
	}
	evt0 := act.GetTimeline()[0]
	if evt0.GetId() != "evt-1" {
		t.Errorf("timeline[0].id = %q, want evt-1", evt0.GetId())
	}
	if evt0.GetCreatedAtUnixMs() != 1000 {
		t.Errorf("timeline[0].created_at_unix_ms = %d, want 1000", evt0.GetCreatedAtUnixMs())
	}
	if evt0.GetKind() != "user_input" {
		t.Errorf("timeline[0].kind = %q, want user_input", evt0.GetKind())
	}
	if evt0.GetSummary() != "implement feature X" {
		t.Errorf("timeline[0].summary = %q, want 'implement feature X'", evt0.GetSummary())
	}

	evt1 := act.GetTimeline()[1]
	if evt1.GetWorkerKey() != "bootstrap" {
		t.Errorf("timeline[1].worker_key = %q, want bootstrap", evt1.GetWorkerKey())
	}
}

// TestActPayloadRoundTrip_EmptyAct verifies that a node with an empty act state
// (no timeline, no pending_actions) survives the round-trip.
func TestActPayloadRoundTrip_EmptyAct(t *testing.T) {
	original := map[string]*insightifyv1.UiNode{
		"act-empty": {
			Id:   "act-empty",
			Type: insightifyv1.UiNodeType_UI_NODE_TYPE_ACT,
			Act: &insightifyv1.UiActState{
				ActId:  "act-empty",
				Status: insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE,
			},
		},
	}

	marshaled, err := marshalNodes(original)
	if err != nil {
		t.Fatalf("marshalNodes failed: %v", err)
	}

	var intermediate map[string]any
	if err := json.Unmarshal(marshaled, &intermediate); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	restored, err := unmarshalNodes(intermediate)
	if err != nil {
		t.Fatalf("unmarshalNodes failed: %v", err)
	}

	node := restored["act-empty"]
	if node == nil {
		t.Fatal("act-empty not found")
	}
	act := node.GetAct()
	if act == nil {
		t.Fatal("act state is nil")
	}
	if act.GetActId() != "act-empty" {
		t.Errorf("act_id = %q, want act-empty", act.GetActId())
	}
	if act.GetStatus() != insightifyv1.UiActStatus_UI_ACT_STATUS_IDLE {
		t.Errorf("status = %v, want IDLE", act.GetStatus())
	}
	if len(act.GetTimeline()) != 0 {
		t.Errorf("timeline should be empty, got %d", len(act.GetTimeline()))
	}
	if len(act.GetPendingActions()) != 0 {
		t.Errorf("pending_actions should be empty, got %d", len(act.GetPendingActions()))
	}
}

// TestActPayloadProtojsonFieldNames checks that protojson emits proto_names
// (snake_case) which can be re-parsed correctly.
func TestActPayloadProtojsonFieldNames(t *testing.T) {
	node := &insightifyv1.UiNode{
		Id:   "n1",
		Type: insightifyv1.UiNodeType_UI_NODE_TYPE_ACT,
		Act: &insightifyv1.UiActState{
			ActId:          "a1",
			SelectedWorker: "bootstrap",
			Timeline: []*insightifyv1.UiActTimelineEvent{
				{Id: "e1", CreatedAtUnixMs: 12345, WorkerKey: "wk"},
			},
		},
	}

	opts := protojson.MarshalOptions{UseProtoNames: true}
	data, err := opts.Marshal(node)
	if err != nil {
		t.Fatalf("protojson marshal failed: %v", err)
	}
	jsonStr := string(data)

	// Check snake_case field names
	for _, want := range []string{"act_id", "selected_worker", "created_at_unix_ms", "worker_key"} {
		if !containsSubstring(jsonStr, want) {
			t.Errorf("expected field name %q in JSON output, got: %s", want, jsonStr)
		}
	}

	// Verify re-parse
	parsed := &insightifyv1.UiNode{}
	if err := protojson.Unmarshal(data, parsed); err != nil {
		t.Fatalf("protojson unmarshal failed: %v", err)
	}
	if parsed.GetAct().GetActId() != "a1" {
		t.Errorf("act_id after re-parse = %q, want a1", parsed.GetAct().GetActId())
	}
	if parsed.GetAct().GetTimeline()[0].GetCreatedAtUnixMs() != 12345 {
		t.Errorf("created_at_unix_ms = %d, want 12345", parsed.GetAct().GetTimeline()[0].GetCreatedAtUnixMs())
	}
}

// TestLegacyLlmChatFieldIsIgnored ensures old persisted payloads that still
// include llm_chat do not break after removing that field from proto.
func TestLegacyLlmChatFieldIsIgnored(t *testing.T) {
	legacy := map[string]any{
		"act-legacy": map[string]any{
			"id":   "act-legacy",
			"type": 5,
			"meta": map[string]any{
				"title": "Legacy",
			},
			"act": map[string]any{
				"act_id": "act-legacy",
				"status": 2,
				"mode":   "planning",
				"goal":   "migrate",
			},
			"llm_chat": map[string]any{
				"model":         "Low",
				"is_responding": false,
				"messages": []map[string]any{
					{"id": "m1", "role": 2, "content": "legacy"},
				},
			},
		},
	}

	nodes, err := unmarshalNodes(legacy)
	if err != nil {
		t.Fatalf("unmarshalNodes should ignore llm_chat, got error: %v", err)
	}
	node := nodes["act-legacy"]
	if node == nil {
		t.Fatal("act-legacy not found")
	}
	if node.GetType() != insightifyv1.UiNodeType_UI_NODE_TYPE_ACT {
		t.Fatalf("node type = %v, want ACT", node.GetType())
	}
	if node.GetAct() == nil || node.GetAct().GetActId() != "act-legacy" {
		t.Fatalf("act payload missing or broken: %+v", node.GetAct())
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
