package codebase

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"insightify/internal/artifact"
	"insightify/internal/llm/middleware"
	llmclient "insightify/internal/llm/client"
	"insightify/internal/llm/tool"
	"insightify/internal/common/safeio"
	"insightify/internal/common/scheduler"
)

type codeSymbolsOutput struct {
	Files []struct {
		Path        string                      `json:"path"`
		Identifiers []artifact.IdentifierSignal `json:"identifiers"`
	} `json:"files"`
}

var codeSymbolsPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Extract identifiers and their implementation spans for each provided file.",
	Background:   "Worker CodeSymbols analyzes code snippets to identify defined symbols and their dependencies.",
	OutputFields: llmtool.MustFieldsFromStruct(codeSymbolsOutput{}),
	Constraints: []string{
		"Describe only concrete identifiers defined in each file.",
		"Return every input file once; if no identifiers exist, return an empty list.",
		"Start <= end; omit duplicates.",
		"Use 'lines': null when unknown.",
		"Scope.level must be one of local|file|module|package|repository.",
	},
	Rules: []string{
		"For each identifier, add a natural language summary of what the identifier does.",
		"Summary detail scales with span length: >20 lines -> richer summary, <=5 lines -> concise or empty.",
		"If no summary is provided, omit notes as well.",
		"For each identifier, list the identifiers it requires/uses in 'requires' with both path and identifier name when known.",
		"Classify each requirement as user|library|runtime|vendor|stdlib|framework in 'origin'.",
	},
	Assumptions:  []string{"Files provided are source code."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

type CodeSymbols struct {
	LLM llmclient.LLMClient
}

func (p CodeSymbols) Run(ctx context.Context, in artifact.CodeSymbolsIn) (artifact.CodeSymbolsOut, error) {
	if p.LLM == nil {
		return artifact.CodeSymbolsOut{}, fmt.Errorf("codeSymbols: llm client is nil")
	}
	fs := in.RepoFS

	nodes := in.Tasks.Nodes
	for i := range nodes {
		if nodes[i].File.Path == "" && nodes[i].Path != "" {
			nodes[i].File = artifact.NewFileRef(nodes[i].Path)
		}
		if nodes[i].Path == "" {
			nodes[i].Path = nodes[i].File.Path
		}
	}
	results := make([]artifact.IdentifierReport, len(nodes))
	for i, n := range nodes {
		if n.Path != "" {
			results[i].Path = n.Path
		} else {
			results[i].Path = n.File.Path
		}
	}

	var (
		mu    sync.Mutex
		notes = make(map[int][]string)
	)

	runChunk := func(chunkCtx context.Context, chunk []int) (<-chan struct{}, error) {
		ids := append([]int(nil), chunk...)
		totalWeight := 0
		fmt.Printf("codeSymbols chunk schedule: %d nodes\n", len(ids))
		for _, id := range ids {
			if id < 0 || id >= len(nodes) {
				fmt.Printf("  - id=%d (invalid)\n", id)
				continue
			}
			totalWeight += nodes[id].Weight
			fmt.Printf("  - id=%d weight=%d path=%s\n", id, nodes[id].Weight, nodes[id].File.Path)
		}
		if cap := p.LLM.TokenCapacity(); cap > 0 {
			fmt.Printf("  total weight=%d cap=%d\n", totalWeight, cap)
		} else {
			fmt.Printf("  total weight=%d\n", totalWeight)
		}
		ch := make(chan struct{})
		go func() {
			defer close(ch)
			reports, perNodeErr, err := p.processChunk(chunkCtx, in.Repo, fs, nodes, ids)
			if err != nil {
				mu.Lock()
				for _, id := range ids {
					notes[id] = append(notes[id], err.Error())
				}
				mu.Unlock()
			}
			mu.Lock()
			for id, perr := range perNodeErr {
				if perr != nil {
					notes[id] = append(notes[id], perr.Error())
				}
			}
			for _, id := range ids {
				if sigs, ok := reports[id]; ok {
					results[id].Identifiers = sigs
					continue
				}
				if perNodeErr[id] == nil && err == nil {
					notes[id] = append(notes[id], "llm returned no data")
				}
			}
			mu.Unlock()
		}()
		return ch, nil
	}

	targets := make(map[int]struct{}, len(nodes))
	for i := range nodes {
		targets[i] = struct{}{}
	}

	adj := in.Tasks.Adjacency
	weightFn := func(nodeID int) int {
		if nodeID >= 0 && nodeID < len(nodes) {
			if nodes[nodeID].Weight > 0 {
				return nodes[nodeID].Weight
			}
		}
		return 1
	}

	params := scheduler.Params{
		Adj:         adj,
		WeightOf:    scheduler.WeightFn(weightFn),
		Targets:     targets,
		CapPerChunk: p.LLM.TokenCapacity(),
		NParallel:   1,
		Run:         scheduler.ChunkRunner(runChunk),
	}
	if err := scheduler.ScheduleHeavierStart(ctx, params); err != nil {
		return artifact.CodeSymbolsOut{}, err
	}

	for id, ns := range notes {
		results[id].Notes = append(results[id].Notes, ns...)
	}

	return artifact.CodeSymbolsOut{
			Repo:  in.Repo,
			Files: results,
		},
		nil
}

func (p CodeSymbols) processChunk(ctx context.Context, repo string, fs *safeio.SafeFS, nodes []artifact.CodeTasksNode, ids []int) (map[int][]artifact.IdentifierSignal, map[int]error, error) {
	type filePayload struct {
		Path     string `json:"path"`
		Language string `json:"language"`
		Content  string `json:"content"`
	}
	payload := struct {
		Repo  string        `json:"repo"`
		Files []filePayload `json:"files"`
	}{
		Repo: repo,
	}

	perNodeErr := make(map[int]error)
	pathToIDs := make(map[string][]int)

	for _, id := range ids {
		if id < 0 || id >= len(nodes) {
			continue
		}
		node := nodes[id]
		path := node.File.Path
		if path == "" {
			path = node.Path
		}
		if strings.TrimSpace(path) == "" {
			perNodeErr[id] = fmt.Errorf("empty path for node %d", id)
			continue
		}
		data, err := fs.SafeReadFile(filepath.Clean(path))
		if err != nil {
			perNodeErr[id] = fmt.Errorf("read %s: %w", path, err)
			continue
		}
		payload.Files = append(payload.Files, filePayload{
			Path:     path,
			Language: strings.TrimPrefix(filepath.Ext(path), "."),
			Content:  string(data),
		})
		pathToIDs[path] = append(pathToIDs[path], id)
	}

	if len(payload.Files) == 0 {
		return nil, perNodeErr, nil
	}

	// Build prompt using llmtool
	prompt, err := llmtool.StructuredPromptBuilder(codeSymbolsPromptSpec)(ctx, &llmtool.ToolState{Input: payload}, nil)
	if err != nil {
		return nil, perNodeErr, err
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, perNodeErr, fmt.Errorf("encode payload: %w", err)
	}
	fmt.Printf("codeSymbols chunk: files=%d tokens=%d\n", len(payload.Files), llmclient.CountTokens(string(payloadBytes)))

	raw, err := p.LLM.GenerateJSON(llm.WithWorker(ctx, "codeSymbols"), prompt, payload)
	if err != nil {
		return nil, perNodeErr, err
	}

	var parsed codeSymbolsOutput
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, perNodeErr, err
	}

	reports := make(map[int][]artifact.IdentifierSignal)
	for _, file := range parsed.Files {
		idsForPath := pathToIDs[file.Path]
		if len(idsForPath) == 0 {
			continue
		}
		sigs := make([]artifact.IdentifierSignal, len(file.Identifiers))
		copy(sigs, file.Identifiers)
		for i := range sigs {
			if sigs[i].Scope.Level == "" {
				sigs[i].Scope.Level = "file"
			}
		}
		for _, id := range idsForPath {
			reports[id] = append([]artifact.IdentifierSignal(nil), sigs...)
		}
	}

	return reports, perNodeErr, nil
}
