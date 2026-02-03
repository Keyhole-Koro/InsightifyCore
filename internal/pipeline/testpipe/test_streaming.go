package testpipe

import (
	"context"
	"time"

	pipelinev1 "insightify/gen/go/pipeline/v1"
)

// StreamStep represents a step in the test streaming pipeline.
type StreamStep struct {
	Message  string
	Progress int32
	Delay    time.Duration
}

// TestStreamingPipeline is a mock pipeline that simulates streaming progress.
type TestStreamingPipeline struct{}

// Steps returns the predefined steps for the test pipeline.
func (p *TestStreamingPipeline) Steps() []StreamStep {
	return []StreamStep{
		{Message: "Step 1: Initializing...", Progress: 10, Delay: 1 * time.Second},
		{Message: "Step 2: Loading Graph...", Progress: 30, Delay: 2 * time.Second},
		{Message: "Step 3: Analyzing nodes...", Progress: 50, Delay: 2 * time.Second},
		{Message: "Step 4: Computing edges...", Progress: 70, Delay: 1500 * time.Millisecond},
		{Message: "Step 5: Finalizing...", Progress: 90, Delay: 1 * time.Second},
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

	for _, step := range p.Steps() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case progressCh <- step:
		}
		time.Sleep(step.Delay)
	}

	return p.GenerateSampleGraph(), nil
}
