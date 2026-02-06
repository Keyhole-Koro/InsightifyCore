package utils

import (
	"strings"
	"testing"

	pipelinev1 "insightify/gen/go/pipeline/v1"
)

func TestAssignGraphNodeUIDs_RewritesNodeIDsAndEdges(t *testing.T) {
	view := &pipelinev1.ClientView{
		Graph: &pipelinev1.GraphView{
			Nodes: []*pipelinev1.GraphNode{
				{Id: "a", Label: "A"},
				{Id: "a", Label: "A-dup"},
				{Id: "", Label: "B"},
			},
			Edges: []*pipelinev1.GraphEdge{
				{From: "a", To: "a"},
			},
		},
	}

	m := AssignGraphNodeUIDs(view)
	if len(m) == 0 {
		t.Fatalf("expected non-empty id map")
	}

	n0 := view.Graph.Nodes[0].GetId()
	n1 := view.Graph.Nodes[1].GetId()
	n2 := view.Graph.Nodes[2].GetId()
	if n0 == "" || n1 == "" || n2 == "" {
		t.Fatalf("all node IDs must be assigned")
	}
	if n0 == "a" || n1 == "a" {
		t.Fatalf("expected node IDs to be rewritten: %q %q", n0, n1)
	}
	if n0 == n1 || n1 == n2 || n0 == n2 {
		t.Fatalf("node IDs must be unique: %q %q %q", n0, n1, n2)
	}

	e := view.Graph.Edges[0]
	if e.From == "a" || e.To == "a" {
		t.Fatalf("expected edge endpoints rewritten, got from=%q to=%q", e.From, e.To)
	}
}

func TestAssignGraphNodeUIDs_DeterministicPattern(t *testing.T) {
	view := &pipelinev1.ClientView{
		Graph: &pipelinev1.GraphView{
			Nodes: []*pipelinev1.GraphNode{
				{Id: "node-main", Label: "Node Main"},
			},
		},
	}

	AssignGraphNodeUIDs(view)
	got := view.Graph.Nodes[0].GetId()
	if !strings.HasPrefix(got, "node-main-") {
		t.Fatalf("unexpected uid format: %q", got)
	}
}

func TestAssignGraphNodeUIDsWithGenerator_StableAcrossSnapshots(t *testing.T) {
	gen := NewUIDGenerator()
	view1 := &pipelinev1.ClientView{
		Graph: &pipelinev1.GraphView{
			Nodes: []*pipelinev1.GraphNode{
				{Id: "init", Label: "Initialize"},
				{Id: "load", Label: "Load"},
			},
			Edges: []*pipelinev1.GraphEdge{
				{From: "init", To: "load"},
			},
		},
	}
	view2 := &pipelinev1.ClientView{
		Graph: &pipelinev1.GraphView{
			Nodes: []*pipelinev1.GraphNode{
				{Id: "init", Label: "Initialize"},
				{Id: "load", Label: "Load"},
			},
			Edges: []*pipelinev1.GraphEdge{
				{From: "init", To: "load"},
			},
		},
	}

	AssignGraphNodeUIDsWithGenerator(gen, view1)
	uidInit1 := view1.Graph.Nodes[0].GetId()
	uidLoad1 := view1.Graph.Nodes[1].GetId()

	AssignGraphNodeUIDsWithGenerator(gen, view2)
	uidInit2 := view2.Graph.Nodes[0].GetId()
	uidLoad2 := view2.Graph.Nodes[1].GetId()

	if uidInit1 != uidInit2 || uidLoad1 != uidLoad2 {
		t.Fatalf("uids must stay stable across snapshots: (%q,%q) vs (%q,%q)", uidInit1, uidLoad1, uidInit2, uidLoad2)
	}
	if view2.Graph.Edges[0].From != uidInit2 || view2.Graph.Edges[0].To != uidLoad2 {
		t.Fatalf("edge should follow stable rewritten ids: %+v", view2.Graph.Edges[0])
	}
}
