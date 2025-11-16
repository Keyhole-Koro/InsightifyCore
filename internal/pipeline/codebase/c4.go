package codebase

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/safeio"
	"insightify/internal/scheduler"
	cb "insightify/internal/types/codebase"
)

const promptC4 = `You extract identifiers and their implementation spans for each provided file.

Input JSON:
{
  "repo": "<repository name>",
  "files": [
    {
      "path": "<relative file path>",
      "language": "<language or extension>",
      "content": "<full file text>"
    }
  ]
}

Output STRICT JSON:
{
  "files": [
    {
      "path": "<relative file path>",
      "identifiers": [
        {
          "name": "string",
          "role": "string",                 // function, class, handler, etc.
          "lines": [start, end],            // 1-based inclusive line numbers
          "scope": {
            "level": "local|file|module|package|repository",
            "access": "string",             // describe visibility (e.g., exported, private)
            "notes": "string"               // optional guidance
          }
        }
      ]
    }
  ]
}

Rules:
- Describe only concrete identifiers defined in each file.
- Return every input file once; if no identifiers exist, return an empty list.
- Start <= end; omit duplicates.
- Use "lines": null when unknown.
- Scope.level must be one of local|file|module|package|repository.`

type C4 struct {
	LLM llmclient.LLMClient
}

func (p C4) Run(ctx context.Context, in cb.C4In) (cb.C4Out, error) {
	if p.LLM == nil {
		return cb.C4Out{}, fmt.Errorf("c4: llm client is nil")
	}
	fs := in.RepoFS

	nodes := in.Tasks.Nodes
	results := make([]cb.IdentifierReport, len(nodes))
	for i, n := range nodes {
		results[i].Path = n.Path
	}

	var (
		mu    sync.Mutex
		notes = make(map[int][]string)
	)

	runChunk := func(chunkCtx context.Context, chunk []int) (<-chan struct{}, error) {
		ids := append([]int(nil), chunk...)
		totalWeight := 0
		fmt.Printf("c4 chunk schedule: %d nodes\n", len(ids))
		for _, id := range ids {
			if id < 0 || id >= len(nodes) {
				fmt.Printf("  - id=%d (invalid)\n", id)
				continue
			}
			totalWeight += nodes[id].Weight
			fmt.Printf("  - id=%d weight=%d path=%s\n", id, nodes[id].Weight, nodes[id].Path)
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
		return cb.C4Out{}, err
	}

	for id, ns := range notes {
		results[id].Notes = append(results[id].Notes, ns...)
	}

	return cb.C4Out{
		Repo:  in.Repo,
		Files: results,
	}, nil
}

func (p C4) processChunk(ctx context.Context, repo string, fs *safeio.SafeFS, nodes []cb.C3Node, ids []int) (map[int][]cb.IdentifierSignal, map[int]error, error) {
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
		data, err := fs.SafeReadFile(filepath.Clean(node.Path))
		if err != nil {
			perNodeErr[id] = fmt.Errorf("read %s: %w", node.Path, err)
			continue
		}
		payload.Files = append(payload.Files, filePayload{
			Path:     node.Path,
			Language: strings.TrimPrefix(filepath.Ext(node.Path), "."),
			Content:  string(data),
		})
		pathToIDs[node.Path] = append(pathToIDs[node.Path], id)
	}

	if len(payload.Files) == 0 {
		return nil, perNodeErr, nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, perNodeErr, fmt.Errorf("encode payload: %w", err)
	}
	fmt.Printf("c4 chunk: files=%d tokens=%d\n", len(payload.Files), llmclient.CountTokens(string(payloadBytes)))

	raw, err := p.LLM.GenerateJSON(llm.WithPhase(ctx, "c4"), promptC4, payload)
	if err != nil {
		return nil, perNodeErr, err
	}

	var parsed struct {
		Files []struct {
			Path        string                `json:"path"`
			Identifiers []cb.IdentifierSignal `json:"identifiers"`
		} `json:"files"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, perNodeErr, err
	}

	reports := make(map[int][]cb.IdentifierSignal)
	for _, file := range parsed.Files {
		idsForPath := pathToIDs[file.Path]
		if len(idsForPath) == 0 {
			continue
		}
		sigs := make([]cb.IdentifierSignal, len(file.Identifiers))
		copy(sigs, file.Identifiers)
		for i := range sigs {
			if sigs[i].Scope.Level == "" {
				sigs[i].Scope.Level = "file"
			}
		}
		for _, id := range idsForPath {
			reports[id] = append([]cb.IdentifierSignal(nil), sigs...)
		}
	}

	return reports, perNodeErr, nil
}
