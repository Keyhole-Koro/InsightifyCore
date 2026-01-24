package codebase

import (
	"context"
	"path/filepath"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
)

type C3 struct {
	LLM llmclient.LLMClient
}

func (p C3) Run(ctx context.Context, in artifact.C3In) (artifact.C3Out, error) {
	_ = ctx
	graph := in.Graph
	fs := in.RepoFS

	weights := make([]int, len(graph.Nodes))
	for i, node := range graph.Nodes {
		data, err := fs.SafeReadFile(filepath.Clean(node.File.Path))
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

	taskNodes := make([]artifact.C3Node, len(graph.Nodes))
	for i, node := range graph.Nodes {
		taskNodes[i] = artifact.C3Node{
			ID:       node.ID,
			Path:     node.File.Path,
			File:     node.File,
			TaskType: "llm_api",
			Weight:   weights[i],
		}
	}

	adj := make([][]int, len(graph.Adjacency))
	for i := range graph.Adjacency {
		adj[i] = append([]int(nil), graph.Adjacency[i]...)
	}

	return artifact.C3Out{
		Repo:        in.Repo,
		CapPerChunk: in.CapPerChunk,
		Nodes:       taskNodes,
		Adjacency:   adj,
	}, nil
}