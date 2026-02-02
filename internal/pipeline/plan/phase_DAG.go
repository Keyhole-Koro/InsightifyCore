package plan

import (
	"context"

	pipelinev1 "insightify/gen/go/pipeline/v1"
	"insightify/internal/artifact"
)

// PlanContext holds dependencies for the planning phase.
type PlanContext struct {
	// LLM client or other dependencies can be added here if needed for more complex planning
	LLM any
}

func (p *PlanContext) Run(ctx context.Context, in artifact.PlanDependenciesIn) (artifact.PlanDependenciesOut, error) {
	nodes := make([]*pipelinev1.GraphNode, 0, len(in.Phases))
	edges := make([]*pipelinev1.GraphEdge, 0)

	for _, ph := range in.Phases {
		nodes = append(nodes, &pipelinev1.GraphNode{
			Id:          ph.Key,
			Label:       ph.Key,
			Description: ph.Description,
		})
		for _, req := range ph.Requires {
			edges = append(edges, &pipelinev1.GraphEdge{
				From: req,
				To:   ph.Key,
			})
		}
	}

	return artifact.PlanDependenciesOut{
		RuntimeState: in, // Persist input as state for now
		ClientView: &pipelinev1.ClientView{
			Graph: &pipelinev1.GraphView{
				Nodes: nodes,
				Edges: edges,
			},
		},
	}, nil
}
