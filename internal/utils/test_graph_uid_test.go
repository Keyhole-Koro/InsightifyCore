package utils

import (
	"strings"
	"testing"

	pipelinev1 "insightify/gen/go/pipeline/v1"
)

func TestAssignGraphNodeUIDs_RewritesNodeIDsAndEdges(t *testing.T) {
	view := &pipelinev1.ClientView{
		Content: &pipelinev1.ClientView_Graph{
			Graph: &pipelinev1.GraphView{
				Nodes: []*pipelinev1.GraphNode{
					{Uid: "a", Label: "A"},
					{Uid: "a", Label: "A-dup"},
					{Uid: "", Label: "B"},
				},
				Edges: []*pipelinev1.GraphEdge{
					{From: "a", To: "a"},
				},
			},
		},
	}

	m := AssignGraphNodeUIDs(view)
	if len(m) == 0 {
		t.Fatalf("expected non-empty id map")
	}

	graph := view.GetGraph()
	n0 := graph.Nodes[0].GetUid()
	n1 := graph.Nodes[1].GetUid()
	n2 := graph.Nodes[2].GetUid()
	if n0 == "" || n1 == "" || n2 == "" {
		t.Fatalf("all node IDs must be assigned")
	}
	if n0 == "a" || n1 == "a" {
		t.Fatalf("expected node IDs to be rewritten: %q %q", n0, n1)
	}
	if n0 == n1 || n1 == n2 || n0 == n2 {
		t.Fatalf("node IDs must be unique: %q %q %q", n0, n1, n2)
	}

	e := graph.Edges[0]
	if e.From == "a" || e.To == "a" {
		t.Fatalf("expected edge endpoints rewritten, got from=%q to=%q", e.From, e.To)
	}
}

func TestAssignGraphNodeUIDs_DeterministicPattern(t *testing.T) {
	view := &pipelinev1.ClientView{
		Content: &pipelinev1.ClientView_Graph{
			Graph: &pipelinev1.GraphView{
				Nodes: []*pipelinev1.GraphNode{
					{Uid: "node-main", Label: "Node Main"},
				},
			},
		},
	}

	AssignGraphNodeUIDs(view)
	got := view.GetGraph().Nodes[0].GetUid()
	if !strings.HasPrefix(got, "node-main-") {
		t.Fatalf("unexpected uid format: %q", got)
	}
}

func TestAssignGraphNodeUIDsWithGenerator_StableAcrossSnapshots(t *testing.T) {
	gen := NewUIDGenerator()
	view1 := &pipelinev1.ClientView{
		Content: &pipelinev1.ClientView_Graph{
			Graph: &pipelinev1.GraphView{
				Nodes: []*pipelinev1.GraphNode{
					{Uid: "init", Label: "Initialize"},
					{Uid: "load", Label: "Load"},
				},
				Edges: []*pipelinev1.GraphEdge{
					{From: "init", To: "load"},
				},
			},
		},
	}
	view2 := &pipelinev1.ClientView{
		Content: &pipelinev1.ClientView_Graph{
			Graph: &pipelinev1.GraphView{
				Nodes: []*pipelinev1.GraphNode{
					{Uid: "init", Label: "Initialize"},
					{Uid: "load", Label: "Load"},
				},
				Edges: []*pipelinev1.GraphEdge{
					{From: "init", To: "load"},
				},
			},
		},
	}

	AssignGraphNodeUIDsWithGenerator(gen, view1)
	uidInit1 := view1.GetGraph().Nodes[0].GetUid()
	uidLoad1 := view1.GetGraph().Nodes[1].GetUid()

	AssignGraphNodeUIDsWithGenerator(gen, view2)
	uidInit2 := view2.GetGraph().Nodes[0].GetUid()
	uidLoad2 := view2.GetGraph().Nodes[1].GetUid()

	if uidInit1 != uidInit2 || uidLoad1 != uidLoad2 {
		t.Fatalf("uids must stay stable across snapshots: (%q,%q) vs (%q,%q)", uidInit1, uidLoad1, uidInit2, uidLoad2)
	}
	if view2.GetGraph().Edges[0].From != uidInit2 || view2.GetGraph().Edges[0].To != uidLoad2 {
		t.Fatalf("edge should follow stable rewritten ids: %+v", view2.GetGraph().Edges[0])
	}
}
