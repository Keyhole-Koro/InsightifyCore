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
		Graph: &pipelinev1.GraphView{
			Nodes: []*pipelinev1.GraphNode{
				{Id: "init", Label: "Initialize", Description: "System initialization"},
				{Id: "load", Label: "Load Data", Description: "Loading input data"},
				{Id: "process", Label: "Process", Description: "Main processing step"},
				{Id: "output", Label: "Output", Description: "Generate output"},
			},
			Edges: []*pipelinev1.GraphEdge{
				{From: "init", To: "load"},
				{From: "load", To: "process"},
				{From: "process", To: "output"},
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
		Graph: &pipelinev1.GraphView{},
	}

	for i, step := range p.Steps() {
		if fullView != nil && fullView.Graph != nil && partialView.Graph != nil {
			if i < len(fullView.Graph.Nodes) {
				partialView.Graph.Nodes = append(partialView.Graph.Nodes, fullView.Graph.Nodes[i])
			}

			edgeIndex := i - 1
			if edgeIndex >= 0 && edgeIndex < len(fullView.Graph.Edges) {
				partialView.Graph.Edges = append(partialView.Graph.Edges, fullView.Graph.Edges[edgeIndex])
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
	if view.Graph == nil {
		return cloned
	}

	cloned.Graph = &pipelinev1.GraphView{}
	if len(view.Graph.Nodes) > 0 {
		cloned.Graph.Nodes = make([]*pipelinev1.GraphNode, 0, len(view.Graph.Nodes))
		for _, n := range view.Graph.Nodes {
			if n == nil {
				cloned.Graph.Nodes = append(cloned.Graph.Nodes, nil)
				continue
			}
			cp := *n
			cloned.Graph.Nodes = append(cloned.Graph.Nodes, &cp)
		}
	}
	if len(view.Graph.Edges) > 0 {
		cloned.Graph.Edges = make([]*pipelinev1.GraphEdge, 0, len(view.Graph.Edges))
		for _, e := range view.Graph.Edges {
			if e == nil {
				cloned.Graph.Edges = append(cloned.Graph.Edges, nil)
				continue
			}
			cp := *e
			cloned.Graph.Edges = append(cloned.Graph.Edges, &cp)
		}
	}
	return cloned
}
