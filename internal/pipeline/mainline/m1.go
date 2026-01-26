package mainline

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/delta"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/scan"
	"insightify/internal/utils"
)

type m1DeltaOut struct {
	Delta delta.Delta `json:"delta" prompt_desc:"Changes vs previous hypothesis (added, removed, modified)."`
}

var (
	reStripMD   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reStripHTML = regexp.MustCompile(`(?is)<img[^>]*>`)
)

var m1PromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Update the architecture hypothesis by emitting only the delta versus the previous version.",
	Background:   "Phase M1 maintains a hypothesis and refines it using MCP tools and evidence. It never re-emits the full hypothesis.",
	OutputFields: llmtool.MustFieldsFromStruct(m1DeltaOut{}),
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

type M1 struct {
	LLM   llmclient.LLMClient
	Tools llmtool.ToolProvider
}

// Run now accepts a single M1In to mirror M1's API.
func (p *M1) Run(ctx context.Context, in artifact.M1In) (artifact.M1Out, error) {
	if p == nil || p.LLM == nil {
		return artifact.M1Out{}, fmt.Errorf("m1: llm client is nil")
	}
	if p.Tools == nil {
		return artifact.M1Out{}, fmt.Errorf("m1: tools registry is nil")
	}

	// Calculate IgnoreDirs within Run (part of Elimination)
	ignoreDirs := utils.UniqueStrings(utils.BaseNames(in.LibraryRoots...)...)

	if len(in.FileIndex) == 0 || len(in.MDDocs) == 0 {
		// Use calculated ignoreDirs
		idx, mds := scanForM1(in.Repo, ignoreDirs)
		if len(in.FileIndex) == 0 {
			in.FileIndex = idx
		}
		if len(in.MDDocs) == 0 {
			in.MDDocs = mds
		}
	}
	hints := in.Hints
	if hints == nil {
		hints = &artifact.M1Hints{}
	}
	state := defaultM1Out()

	// Eliminate noise from documents just before prompt construction
	promptDocs := make([]artifact.MDDoc, len(in.MDDocs))
	for i, d := range in.MDDocs {
		promptDocs[i] = artifact.MDDoc{
			Path: d.Path,
			Text: eliminateNoise(d.Text),
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

		raw, _, err := loop.Run(ctx, input, llmtool.StructuredPromptBuilder(m1PromptSpec))
		if err != nil {
			return artifact.M1Out{}, err
		}
		var step m1DeltaOut
		if err := json.Unmarshal(raw, &step); err != nil {
			return artifact.M1Out{}, fmt.Errorf("M1 JSON invalid: %w", err)
		}
		delta.Normalize(&step.Delta)
		next, err := applyM1Delta(state, step.Delta)
		if err != nil {
			return artifact.M1Out{}, err
		}
		state = next
		if isEmptyDelta(step.Delta) {
			break
		}
	}
	return state, nil
}

func defaultM1Out() artifact.M1Out {
	return artifact.M1Out{
		ArchitectureHypothesis: artifact.M1Hypothesis{
			Purpose:        "",
			Summary:        "",
			KeyComponents:  []artifact.M1KeyComponent{},
			ExecutionModel: "",
			TechStack: artifact.M1TechStack{
				Platforms:  []string{},
				Languages:  []string{},
				BuildTools: []string{},
			},
			Assumptions: []string{},
			Unknowns:    []string{},
			Confidence:  0,
		},
		Contradictions: []artifact.M1Contradiction{},
	}
}

func applyM1Delta(state artifact.M1Out, d delta.Delta) (artifact.M1Out, error) {
	updated, err := delta.Apply(state, d)
	if err != nil {
		return artifact.M1Out{}, err
	}

	nb, _ := json.Marshal(updated)
	var out artifact.M1Out
	if err := json.Unmarshal(nb, &out); err != nil {
		return artifact.M1Out{}, err
	}
	return out, nil
}

func eliminateNoise(txt string) string {
	txt = reStripMD.ReplaceAllString(txt, "")
	txt = reStripHTML.ReplaceAllString(txt, "")
	return txt
}

func isEmptyDelta(d delta.Delta) bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Modified) == 0
}

func scanForM1(repo string, ignore []string) ([]artifact.FileIndexEntry, []artifact.MDDoc) {
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
