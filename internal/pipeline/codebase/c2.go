package codebase

import (
	"container/heap"
	"context"
	"sort"

	cb "insightify/internal/types/codebase"
)

type C2 struct{}

// Run builds a directed dependency graph from C1 output with normalized nodes.
// It collapses bidirectional edges (keeping the heavier direction) and ensures
// the resulting graph is acyclic so later stages do not repeat that work.
func (C2) Run(ctx context.Context, in cb.C2In) (cb.C2Out, error) {
	_ = ctx

	pathToRef := make(map[string]cb.FileRef)
	register := func(ref cb.FileRef) {
		if ref.Path == "" {
			return
		}
		if _, ok := pathToRef[ref.Path]; !ok {
			pathToRef[ref.Path] = ref
		}
	}

	for _, dep := range in.Dependencies {
		for _, sd := range dep.Files {
			register(sd.File)
			for _, req := range sd.Requires {
				register(req)
			}
		}
	}

	paths := make([]string, 0, len(pathToRef))
	for p := range pathToRef {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	idByPath := make(map[string]int, len(paths))
	nodes := make([]cb.DependencyNode, len(paths))
	for i, p := range paths {
		idByPath[p] = i
		nodes[i] = cb.DependencyNode{
			ID:   i,
			File: pathToRef[p],
		}
	}

	edgeCounts := make(map[int]map[int]int)
	addEdge := func(from, to int) {
		if from == to {
			return
		}
		if edgeCounts[from] == nil {
			edgeCounts[from] = make(map[int]int)
		}
		edgeCounts[from][to]++
	}

	for _, dep := range in.Dependencies {
		for _, sd := range dep.Files {
			fromID := idByPath[sd.File.Path]
			for _, req := range sd.Requires {
				depID := idByPath[req.Path]
				addEdge(depID, fromID)
			}
		}
	}

	for from, tos := range edgeCounts {
		for to, cnt := range tos {
			if back, ok := edgeCounts[to][from]; ok {
				if back > cnt || (back == cnt && nodes[to].File.Path < nodes[from].File.Path) {
					delete(edgeCounts[from], to)
				} else {
					delete(edgeCounts[to], from)
				}
			}
		}
	}

	adjMaps := make([]map[int]struct{}, len(nodes))
	for from, tos := range edgeCounts {
		for to := range tos {
			if adjMaps[from] == nil {
				adjMaps[from] = make(map[int]struct{})
			}
			adjMaps[from][to] = struct{}{}
		}
	}

	breakCycles(adjMaps)

	adjacency := make([][]int, len(nodes))
	for i, m := range adjMaps {
		for to := range m {
			adjacency[i] = append(adjacency[i], to)
		}
		sort.Ints(adjacency[i])
	}

	return cb.C2Out{
		Repo: in.Repo,
		Graph: cb.DependencyGraph{
			Nodes:     nodes,
			Adjacency: adjacency,
		},
	}, nil
}

// breakCycles removes edges to ensure the adjacency map encodes a DAG.
func breakCycles(adj []map[int]struct{}) {
	n := len(adj)
	if n == 0 {
		return
	}

	indeg := make([]int, n)
	reverse := make([]map[int]struct{}, n)
	for from := range adj {
		for to := range adj[from] {
			indeg[to]++
			if reverse[to] == nil {
				reverse[to] = make(map[int]struct{})
			}
			reverse[to][from] = struct{}{}
		}
	}

	remain := make(map[int]struct{}, n)
	for i := 0; i < n; i++ {
		remain[i] = struct{}{}
	}

	h := &intHeap{}
	heap.Init(h)
	for i := 0; i < n; i++ {
		if indeg[i] == 0 {
			heap.Push(h, i)
		}
	}

	for len(remain) > 0 {
		var curr int
		if h.Len() > 0 {
			curr = heap.Pop(h).(int)
		} else {
			curr = smallestIndex(remain)
			removeIncomingEdges(curr, adj, reverse, indeg)
			heap.Push(h, curr)
			continue
		}
		if _, ok := remain[curr]; !ok {
			continue
		}
		delete(remain, curr)
		for to := range adj[curr] {
			if reverse[to] != nil {
				delete(reverse[to], curr)
			}
			if indeg[to] > 0 {
				indeg[to]--
				if indeg[to] == 0 {
					heap.Push(h, to)
				}
			}
		}
	}
}

func removeIncomingEdges(node int, adj []map[int]struct{}, reverse []map[int]struct{}, indeg []int) {
	if node < 0 || node >= len(adj) {
		return
	}
	preds := make([]int, 0, len(reverse[node]))
	for p := range reverse[node] {
		preds = append(preds, p)
	}
	sort.Ints(preds)
	for _, pred := range preds {
		if adj[pred] != nil {
			delete(adj[pred], node)
		}
		delete(reverse[node], pred)
		if indeg[node] > 0 {
			indeg[node]--
		}
	}
	indeg[node] = 0
}

func smallestIndex(m map[int]struct{}) int {
	var smallest int
	first := true
	for k := range m {
		if first || k < smallest {
			smallest = k
			first = false
		}
	}
	return smallest
}

type intHeap []int

func (h intHeap) Len() int           { return len(h) }
func (h intHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *intHeap) Push(x any) {
	*h = append(*h, x.(int))
}

func (h *intHeap) Pop() any {
	old := *h
	n := len(old)
	if n == 0 {
		return 0
	}
	x := old[n-1]
	*h = old[:n-1]
	return x
}
