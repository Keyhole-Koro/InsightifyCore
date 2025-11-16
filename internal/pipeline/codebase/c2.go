package codebase

import (
	"context"
	"sort"
	"strings"

	cb "insightify/internal/types/codebase"
)

type C2 struct{}

// Run builds a directed dependency graph from C1 output.
// Bidirectional edges are collapsed: keep the direction with the larger count; ties default to from<to.
func (C2) Run(ctx context.Context, in cb.C2In) (cb.C2Out, error) {
	_ = ctx
	edgeCounts := make(map[string]map[string]int)
	nodes := make(map[string]struct{})
	reverse := make(map[string]map[string]struct{})

	for _, dep := range in.Dependencies {
		for _, sd := range dep.Dependencies {
			from := normalize(sd.Path)
			nodes[from] = struct{}{}
			for _, req := range sd.Requires {
				to := normalize(req)
				if to == "" || from == "" || to == from {
					continue
				}
				nodes[to] = struct{}{}
				if edgeCounts[from] == nil {
					edgeCounts[from] = make(map[string]int)
				}
				edgeCounts[from][to]++
				if reverse[to] == nil {
					reverse[to] = make(map[string]struct{})
				}
				reverse[to][from] = struct{}{}
			}
		}
	}

	// Resolve bidirectional edges.
	for from, m := range edgeCounts {
		for to, cnt := range m {
			if back, ok := edgeCounts[to][from]; ok {
				// keep the heavier direction; tie-break lexicographically.
				if back > cnt || (back == cnt && strings.Compare(to, from) < 0) {
					delete(edgeCounts[from], to)
				} else {
					delete(edgeCounts[to], from)
				}
			}
		}
	}

	var edges []cb.DependencyEdge
	for from, m := range edgeCounts {
		var tos []string
		for to := range m {
			tos = append(tos, to)
		}
		sort.Strings(tos)
		if len(tos) > 0 {
			edges = append(edges, cb.DependencyEdge{From: from, To: tos})
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		return edges[i].From < edges[j].From
	})

	var dependents []cb.DependencyDependents
	for node := range nodes {
		var deps []string
		for dep := range reverse[node] {
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		dependents = append(dependents, cb.DependencyDependents{Node: node, Dependents: deps})
	}

	sort.Slice(dependents, func(i, j int) bool {
		return dependents[i].Node < dependents[j].Node
	})

	var nodeList []string
	for n := range nodes {
		nodeList = append(nodeList, n)
	}
	sort.Strings(nodeList)

	return cb.C2Out{
		Repo:       in.Repo,
		Nodes:      nodeList,
		Edges:      edges,
		Dependents: dependents,
	}, nil
}

func normalize(p string) string {
	return strings.TrimSpace(p)
}
