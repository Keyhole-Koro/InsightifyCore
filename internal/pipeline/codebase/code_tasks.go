package codebase

import (
	"context"
	"path/filepath"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
)

type CodeTasks struct {
	LLM llmclient.LLMClient
}

func (p CodeTasks) Run(ctx context.Context, in artifact.CodeTasksIn) (artifact.CodeTasksOut, error) {
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

	taskNodes := make([]artifact.CodeTasksNode, len(graph.Nodes))
	for i, node := range graph.Nodes {
		taskNodes[i] = artifact.CodeTasksNode{
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

	return artifact.CodeTasksOut{
		Repo:        in.Repo,
		CapPerChunk: in.CapPerChunk,
		Nodes:       taskNodes,
		Adjacency:   adj,
	}, nil
}