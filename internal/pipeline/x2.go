package pipeline

import (
	"context"
	"fmt"

	"insightify/internal/llm"
	"insightify/internal/scheduler"
	t "insightify/internal/types"
)

type X2 struct {
	LLM         llm.LLMClient
	Broker      llm.PermitBroker
	ReserveWith scheduler.ReservePolicy
}

// BiMap keeps a bidirectional mapping between string keys and dense int IDs.
type BiMap struct {
	s2i map[string]int
	i2s []string
}

func NewBiMap() *BiMap {
	return &BiMap{
		s2i: make(map[string]int),
		i2s: make([]string, 0),
	}
}

// Ensure returns the existing ID for key or assigns a new dense ID [0..n).
func (b *BiMap) Ensure(key string) int {
	if id, ok := b.s2i[key]; ok {
		return id
	}
	id := len(b.i2s)
	b.s2i[key] = id
	b.i2s = append(b.i2s, key)
	return id
}

func (b *BiMap) GetID(key string) (int, bool) {
	id, ok := b.s2i[key]
	return id, ok
}

func (b *BiMap) GetKey(id int) (string, bool) {
	if id < 0 || id >= len(b.i2s) {
		return "", false
	}
	return b.i2s[id], true
}

func (b *BiMap) Size() int { return len(b.i2s) }

// BuildAdjacency builds a dense-ID adjacency from edges.
// - Skips edges with empty From or To (tune policy as needed).
func BuildAdjacency(edges []t.X1Edge) ([][]int, *BiMap) {
	b := NewBiMap()

	// 1) First pass: assign IDs to all endpoints to know final size.
	for _, e := range edges {
		if e.From != "" {
			b.Ensure(e.From)
		}
		if e.To != "" {
			b.Ensure(e.To)
		}
	}

	// 2) Allocate adjacency with the final size.
	n := b.Size()
	adj := make([][]int, n)

	// 3) Second pass: add edges (skip unresolved).
	for _, e := range edges {
		if e.From == "" || e.To == "" {
			continue // skip unresolved externals
		}
		// Edge direction: dependency -> dependent (B -> A when A imports B)
		dep := b.Ensure(e.To)
		depd := b.Ensure(e.From)
		adj[dep] = append(adj[dep], depd)
	}
	return adj, b
}

func WeightTemporary(id int) int {
	// Temporary weight function: all weights are 1.
	return 1
}

func SinksAsTargets(adj [][]int) map[int]struct{} {
	t := make(map[int]struct{})
	for u := range adj {
		if len(adj[u]) == 0 {
			t[u] = struct{}{}
		}
	}
	return t
}

func (x X2) Run(ctx context.Context, in t.X2In) (t.X2Out, error) {

	// Optionally invoke scheduler to reserve permits per chunk (no-op if Broker nil)
	adj, _ := BuildAdjacency(in.Dependencies)
	targets := SinksAsTargets(adj)
	runner := func(ctx context.Context, chunk []int) (<-chan struct{}, error) {
		ch := make(chan struct{})
		close(ch)
		return ch, nil
	}
	if err := scheduler.ScheduleHeavierStart(ctx, scheduler.Params{
		Adj:         adj,
		WeightOf:    WeightTemporary,
		Targets:     targets,
		CapPerChunk: 5,
		NParallel:   2,
		Run:         runner,
		Broker:      x.Broker,
		ReserveWith: x.ReserveWith,
	}); err != nil {
		return t.X2Out{}, fmt.Errorf("schedule failed: %w", err)
	}

	return t.X2Out{Sorted: nodes, Files: files}, nil
}

func setKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
