package pipeline

import (
	"context"
	"sort"

	"insightify/internal/llm"
	t "insightify/internal/types"
)

type P4Reduce struct{ LLM llm.LLMClient }

// Deterministic merge w/o LLM for now.
func (p *P4Reduce) Run(ctx context.Context, batches []t.P4Out) (t.P4Out, error) {
	merged := t.P4Out{Artifacts: t.Artifacts{}}
	seen := map[string]int{}

	for _, b := range batches {
		for _, e := range b.Edges {
			if idx, ok := seen[e.ID]; ok {
				merged.Edges[idx].Evidence = append(merged.Edges[idx].Evidence, e.Evidence...)
				// simple blend
				merged.Edges[idx].Confidence = (merged.Edges[idx].Confidence + e.Confidence) / 2
			} else {
				seen[e.ID] = len(merged.Edges)
				merged.Edges = append(merged.Edges, e)
			}
		}
		merged.Artifacts.Interfaces = append(merged.Artifacts.Interfaces, b.Artifacts.Interfaces...)
		merged.Artifacts.Schemas = append(merged.Artifacts.Schemas, b.Artifacts.Schemas...)
		merged.Artifacts.Config = append(merged.Artifacts.Config, b.Artifacts.Config...)
	}

	sort.SliceStable(merged.Edges, func(i, j int) bool {
		if merged.Edges[i].Source != merged.Edges[j].Source {
			return merged.Edges[i].Source < merged.Edges[j].Source
		}
		if merged.Edges[i].Target != merged.Edges[j].Target {
			return merged.Edges[i].Target < merged.Edges[j].Target
		}
		if merged.Edges[i].Type != merged.Edges[j].Type {
			return merged.Edges[i].Type < merged.Edges[j].Type
		}
		return merged.Edges[i].ID < merged.Edges[j].ID
	})

	return merged, nil
}
