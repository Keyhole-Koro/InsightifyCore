package mainline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/common/delta"
	llmclient "insightify/internal/llmclient"
	"insightify/internal/llmtool"
	"insightify/internal/common/scan"
	"insightify/internal/common/utils"
)

type archDesignDeltaOut struct {
	Delta delta.Delta `json:"delta" prompt_desc:"Changes vs previous hypothesis (added, removed, modified)."`
}

var archDesignPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Update the architecture hypothesis by emitting only the delta versus the previous version.",
	Background:   "Phase ArchDesign maintains a hypothesis and refines it using MCP tools and evidence. It never re-emits the full hypothesis.",
	OutputFields: llmtool.MustFieldsFromStruct(archDesignDeltaOut{}),
	Constraints: []string{
		"Use repository-relative paths exactly as provided; never invent paths or filenames.",
		"Evidence must use {path, lines:[start,end]} with 1-based inclusive line numbers; if unknown, set lines to null and explain in notes.",
		"Prefer code over docs when they disagree; report contradictions explicitly.",
		"Do not assume frameworks, stacks, or runtimes unless observed; if inferred, mark as assumption with low confidence.",
		"Do not use fixed vocabularies; use free-form tokens based on evidence.",
		"Do not leak or reuse knowledge outside of the provided inputs or MCP tool results.",
		"Keep names and paths case-sensitive.",
		"Return only delta; do not reprint the full hypothesis.",
		"Use field paths like architecture_hypothesis.summary or architecture_hypothesis.key_components[0].name.",
		"When updating arrays, set the full array in delta.modified.after.",
		"If removing a field, set delta.modified.after to null for that field.",
	},
	Rules: []string{
		"Use MCP tools (scan.list, fs.read, wordidx.search, snippet.collect) to gather evidence before updating.",
		"If inputs are incomplete, request more info by issuing tool calls or returning an empty delta.",
		"When inputs are large, work incrementally: entrypoints, build/manifest, configuration, wiring/adapters, public APIs.",
		"Explicitly mention external nodes/services (APIs, queues, DBs, third-party SaaS) when evidence exists.",
		"If there are no changes, return empty delta arrays.",
	},
	Assumptions: []string{
		"If uncertain, add to architecture_hypothesis.assumptions and reduce confidence.",
		"Unknowns belong in architecture_hypothesis.unknowns.",
	},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent(), llmtool.PresetCautious())

type ArchDesign struct {
	LLM   llmclient.LLMClient
	Tools llmtool.ToolProvider
}

// Run now accepts a single ArchDesignIn to mirror ArchDesign's API.
func (p *ArchDesign) Run(ctx context.Context, in artifact.ArchDesignIn) (artifact.ArchDesignOut, error) {
	if p == nil || p.LLM == nil {
		return artifact.ArchDesignOut{}, fmt.Errorf("archDesign: llm client is nil")
	}
	if p.Tools == nil {
		return artifact.ArchDesignOut{}, fmt.Errorf("archDesign: tools registry is nil")
	}

	// Calculate IgnoreDirs within Run (part of Elimination)
	ignoreDirs := utils.UniqueStrings(utils.BaseNames(in.LibraryRoots...)...)

	if len(in.FileIndex) == 0 || len(in.MDDocs) == 0 {
		// Use calculated ignoreDirs
		idx, mds := scanForArchDesign(in.Repo, ignoreDirs)
		if len(in.FileIndex) == 0 {
			in.FileIndex = idx
		}
		if len(in.MDDocs) == 0 {
			in.MDDocs = mds
		}
	}
	hints := in.Hints
	if hints == nil {
		hints = &artifact.ArchDesignHints{}
	}
	state := defaultArchDesignOut()

	// Eliminate noise from documents just before prompt construction
	promptDocs := make([]artifact.MDDoc, len(in.MDDocs))
	for i, d := range in.MDDocs {
		promptDocs[i] = artifact.MDDoc{
			Path: d.Path,
			Text: utils.MarkDownClean(d.Text),
		}
	}

	const maxOuter = 5
	for i := 0; i < maxOuter; i++ {
		input := map[string]any{
			"previous":       state,
			"file_index":     in.FileIndex,
			"md_docs":        promptDocs,
			"hints":          hints,
			"iteration":      i + 1,
			"max_iterations": maxOuter,
		}

		loop := &llmtool.ToolLoop{
			LLM:      p.LLM,
			Tools:    p.Tools,
			MaxIters: 5,
			Allowed:  []string{"scan.list", "fs.read", "wordidx.search", "snippet.collect", "delta.diff"},
		}

		raw, _, err := loop.Run(ctx, input, llmtool.StructuredPromptBuilder(archDesignPromptSpec))
		if err != nil {
			return artifact.ArchDesignOut{}, err
		}
		var step archDesignDeltaOut
		if err := json.Unmarshal(raw, &step); err != nil {
			return artifact.ArchDesignOut{}, fmt.Errorf("ArchDesign JSON invalid: %w", err)
		}
		delta.Normalize(&step.Delta)
		next, err := applyArchDesignDelta(state, step.Delta)
		if err != nil {
			return artifact.ArchDesignOut{}, err
		}
		state = next
		if isEmptyDelta(step.Delta) {
			break
		}
	}
	return state, nil
}

func defaultArchDesignOut() artifact.ArchDesignOut {
	return artifact.ArchDesignOut{
		ArchitectureHypothesis: artifact.ArchDesignHypothesis{
			Purpose:        "",
			Summary:        "",
			KeyComponents:  []artifact.ArchDesignKeyComponent{},
			ExecutionModel: "",
			TechStack: artifact.ArchDesignTechStack{
				Platforms:  []string{},
				Languages:  []string{},
				BuildTools: []string{},
			},
			Assumptions: []string{},
			Unknowns:    []string{},
			Confidence:  0,
		},
		Contradictions: []artifact.ArchDesignContradiction{},
	}
}

func applyArchDesignDelta(state artifact.ArchDesignOut, d delta.Delta) (artifact.ArchDesignOut, error) {
	updated, err := delta.Apply(state, d)
	if err != nil {
		return artifact.ArchDesignOut{}, err
	}

	nb, _ := json.Marshal(updated)
	var out artifact.ArchDesignOut
	if err := json.Unmarshal(nb, &out); err != nil {
		return artifact.ArchDesignOut{}, err
	}
	return out, nil
}

func isEmptyDelta(d delta.Delta) bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Modified) == 0
}

func scanForArchDesign(repo string, ignore []string) ([]artifact.FileIndexEntry, []artifact.MDDoc) {
	var idx []artifact.FileIndexEntry
	var mds []artifact.MDDoc
	_ = scan.ScanWithOptions(repo, scan.Options{IgnoreDirs: ignore}, func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		idx = append(idx, artifact.FileIndexEntry{Path: f.Path, Size: f.Size})
		if strings.EqualFold(f.Ext, ".md") {
			if b, e := scan.CurrentSafeFS().SafeReadFile(f.AbsPath); e == nil {
				// Keep raw text here
				mds = append(mds, artifact.MDDoc{Path: f.Path, Text: string(b)})
			}
		}
	})
	return idx, mds
}
