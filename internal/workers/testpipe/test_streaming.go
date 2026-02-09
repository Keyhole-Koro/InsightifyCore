package testpipe

import (
	"context"
	"time"

	pipelinev1 "insightify/gen/go/pipeline/v1"
)

// StreamStep represents a step in the test streaming pipeline.
type StreamStep struct {
	Message  string
	Progress float32
	Delay    time.Duration
	View     *pipelinev1.ClientView
}

// TestStreamingPipeline is a mock pipeline that simulates streaming progress.
type TestStreamingPipeline struct{}

// Steps returns the predefined steps for the test pipeline.
func (p *TestStreamingPipeline) Steps() []StreamStep {
	return []StreamStep{
		{Message: "Step 1: Initializing...", Progress: 10, Delay: 300 * time.Millisecond},
		{Message: "Step 2: Loading Graph...", Progress: 30, Delay: 300 * time.Millisecond},
		{Message: "Step 3: Analyzing nodes...", Progress: 50, Delay: 300 * time.Millisecond},
		{Message: "Step 4: Computing edges...", Progress: 70, Delay: 300 * time.Millisecond},
		{Message: "Step 5: Finalizing...", Progress: 90, Delay: 300 * time.Millisecond},
	}
}

// GenerateSampleGraph creates a sample graph for the test result.
func (p *TestStreamingPipeline) GenerateSampleGraph() *pipelinev1.ClientView {
	return &pipelinev1.ClientView{
		Content: &pipelinev1.ClientView_Graph{
			Graph: &pipelinev1.GraphView{
				Nodes: []*pipelinev1.GraphNode{
					{Uid: "init", Label: "Initialize", Description: "System initialization"},
					{Uid: "load", Label: "Load Data", Description: "Loading input data"},
					{Uid: "process", Label: "Process", Description: "Main processing step"},
					{Uid: "output", Label: "Output", Description: "Generate output"},
				},
				Edges: []*pipelinev1.GraphEdge{
					{From: "init", To: "load"},
					{From: "load", To: "process"},
					{From: "process", To: "output"},
				},
			},
		},
	}
}

// Run executes the streaming pipeline, sending progress to the provided channel.
func (p *TestStreamingPipeline) Run(ctx context.Context, progressCh chan<- StreamStep) (*pipelinev1.ClientView, error) {
	defer close(progressCh)

	fullView := p.GenerateSampleGraph()
	partialView := &pipelinev1.ClientView{
		Phase: fullView.GetPhase(),
		Content: &pipelinev1.ClientView_Graph{
			Graph: &pipelinev1.GraphView{},
		},
	}

	for i, step := range p.Steps() {
		fullGraph := fullView.GetGraph()
		partialGraph := partialView.GetGraph()
		if fullGraph != nil && partialGraph != nil {
			if i < len(fullGraph.Nodes) {
				partialGraph.Nodes = append(partialGraph.Nodes, fullGraph.Nodes[i])
			}

			edgeIndex := i - 1
			if edgeIndex >= 0 && edgeIndex < len(fullGraph.Edges) {
				partialGraph.Edges = append(partialGraph.Edges, fullGraph.Edges[edgeIndex])
			}

			step.View = cloneClientView(partialView)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case progressCh <- step:
		}
		time.Sleep(step.Delay)
	}

	return fullView, nil
}

func cloneClientView(view *pipelinev1.ClientView) *pipelinev1.ClientView {
	if view == nil {
		return nil
	}

	cloned := &pipelinev1.ClientView{Phase: view.GetPhase()}
	if view.GetGraph() == nil {
		return cloned
	}

	clonedGraph := &pipelinev1.GraphView{}
	cloned.Content = &pipelinev1.ClientView_Graph{Graph: clonedGraph}
	if len(view.GetGraph().Nodes) > 0 {
		clonedGraph.Nodes = make([]*pipelinev1.GraphNode, 0, len(view.GetGraph().Nodes))
		for _, n := range view.GetGraph().Nodes {
			if n == nil {
				clonedGraph.Nodes = append(clonedGraph.Nodes, nil)
				continue
			}
			cp := *n
			clonedGraph.Nodes = append(clonedGraph.Nodes, &cp)
		}
	}
	if len(view.GetGraph().Edges) > 0 {
		clonedGraph.Edges = make([]*pipelinev1.GraphEdge, 0, len(view.GetGraph().Edges))
		for _, e := range view.GetGraph().Edges {
			if e == nil {
				clonedGraph.Edges = append(clonedGraph.Edges, nil)
				continue
			}
			cp := *e
			clonedGraph.Edges = append(clonedGraph.Edges, &cp)
		}
	}
	return cloned
}
