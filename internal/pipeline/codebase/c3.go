package codebase

import (
	"container/heap"
	"context"
	"path/filepath"
	"sort"

	llmclient "insightify/internal/llmClient"
	cb "insightify/internal/types/codebase"
)

type C3 struct {
	LLM llmclient.LLMClient
}

func (p C3) Run(ctx context.Context, in cb.C3In) (cb.C3Out, error) {
	_ = ctx
	nodes := in.Graph.Nodes
	nodeIndex := make(map[string]int, len(nodes))
	for i, n := range nodes {
		nodeIndex[n] = i
	}

	adjMaps := make([]map[int]struct{}, len(nodes))
	for _, edge := range in.Graph.Edges {
		fromID, ok := nodeIndex[edge.From]
		if !ok {
			continue
		}
		for _, to := range edge.To {
			if depID, ok := nodeIndex[to]; ok && depID != fromID {
				// Scheduler expects dependency -> dependent edges ("dep must run before dependent").
				if adjMaps[depID] == nil {
					adjMaps[depID] = make(map[int]struct{})
				}
				adjMaps[depID][fromID] = struct{}{}
			}
		}
	}

	breakCycles(adjMaps)

	adj := make([][]int, len(nodes))
	for i := range adjMaps {
		for to := range adjMaps[i] {
			adj[i] = append(adj[i], to)
		}
		sort.Ints(adj[i])
	}

	weights := make([]int, len(nodes))
	fs := in.RepoFS

	for i, path := range nodes {
		data, err := fs.SafeReadFile(filepath.Clean(path))
		if err != nil {
			weights[i] = 1
			continue
		}
		count := llmclient.CountTokens(string(data))
		if p.LLM != nil {
			if est := p.LLM.CountTokens(string(data)); est > 0 {
				count = est
			}
		}
		if count <= 0 {
			count = 1
		}
		weights[i] = count
	}

	taskNodes := make([]cb.C3Node, len(nodes))
	for i, path := range nodes {
		taskNodes[i] = cb.C3Node{
			ID:       i,
			Path:     path,
			TaskType: "llm_api",
			Weight:   weights[i],
		}
	}

	return cb.C3Out{
		Repo:        in.Repo,
		CapPerChunk: in.CapPerChunk,
		Nodes:       taskNodes,
		Adjacency:   adj,
	}, nil
}

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
