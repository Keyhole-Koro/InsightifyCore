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
	_ = ctx

	// PlanDependenciesIn contains the phase information via RuntimeState or other fields
	// For now, return a minimal graph that will be populated by the runner
	nodes := make([]*pipelinev1.GraphNode, 0)
	edges := make([]*pipelinev1.GraphEdge, 0)

	return artifact.PlanDependenciesOut{
		RuntimeState: in,
		ClientView: &pipelinev1.ClientView{
			Graph: &pipelinev1.GraphView{
				Nodes: nodes,
				Edges: edges,
			},
		},
	}, nil
}
