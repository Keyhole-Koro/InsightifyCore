package plan

import (
	"context"
	"sort"
	"strings"

	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/artifact"
)

// PlanContext holds dependencies for the planning phase.
type PlanContext struct {
	// LLM client or other dependencies can be added here if needed for more complex planning
	LLM any
}

func (p *PlanContext) Run(ctx context.Context, in artifact.PlanDependenciesIn) (artifact.PlanDependenciesOut, error) {
	_ = ctx

	nodes := make([]*pipelinev1.GraphNode, 0, len(in.Workers))
	edges := make([]*pipelinev1.GraphEdge, 0)

	workersByKey := make(map[string]artifact.WorkerMeta, len(in.Workers))
	for _, w := range in.Workers {
		key := strings.TrimSpace(w.Key)
		if key == "" {
			continue
		}
		w.Key = key
		workersByKey[key] = w
	}

	keys := make([]string, 0, len(workersByKey))
	for key := range workersByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		w := workersByKey[key]
		nodes = append(nodes, &pipelinev1.GraphNode{
			Uid:         w.Key,
			Label:       w.Key,
			Description: strings.TrimSpace(w.Description),
		})
	}

	for _, key := range keys {
		w := workersByKey[key]
		if len(w.Requires) == 0 {
			continue
		}
		reqs := make([]string, 0, len(w.Requires))
		for _, req := range w.Requires {
			r := strings.TrimSpace(req)
			if r == "" {
				continue
			}
			reqs = append(reqs, r)
		}
		sort.Strings(reqs)
		for _, req := range reqs {
			// Create placeholder nodes for required workers not present in input.
			if _, ok := workersByKey[req]; !ok {
				workersByKey[req] = artifact.WorkerMeta{Key: req}
				nodes = append(nodes, &pipelinev1.GraphNode{
					Uid:   req,
					Label: req,
				})
			}
			edges = append(edges, &pipelinev1.GraphEdge{
				From: req,
				To:   key,
			})
		}
	}

	return artifact.PlanDependenciesOut{
		RuntimeState: in,
		ClientView: &pipelinev1.ClientView{
			Phase: "worker_DAG",
			Content: &pipelinev1.ClientView_Graph{
				Graph: &pipelinev1.GraphView{
					Nodes: nodes,
					Edges: edges,
				},
			},
		},
	}, nil
}
