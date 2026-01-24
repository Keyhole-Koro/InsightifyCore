package mainline

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/scan"
	t "insightify/internal/types"
	ml "insightify/internal/types/mainline"
)

var m1PromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Produce an initial architecture hypothesis and propose the next files or patterns to confirm it.",
	Background:   "Phase M1 analyzes the file index and Markdown docs (images and binaries excluded) to draft the system architecture.",
	OutputFields: llmtool.MustFieldsFromStruct(ml.M1Out{}),
	Constraints: []string{
		"Use repository-relative paths exactly as provided; never invent paths or filenames.",
		"Evidence must use {path, lines:[start,end]} with 1-based inclusive line numbers; if unknown, set lines to null and explain in notes.",
		"Prefer code over docs when they disagree; report contradictions explicitly.",
		"Do not assume frameworks, stacks, or runtimes unless observed; if inferred, mark as assumption with low confidence.",
		"Do not use fixed vocabularies; use free-form tokens based on evidence.",
		"Propose at most limits.max_next (default 8) across next_files + next_patterns.",
		"Do not leak or reuse knowledge outside of the provided inputs.",
		"Keep names and paths case-sensitive.",
	},
	Rules: []string{
		"Identify relevant code, cite exact files/symbols, and avoid guessing; if unknown, state it.",
		"If inputs are incomplete, list what else you need under needs_input with exact filenames or glob patterns.",
		"When inputs are large, work incrementally: entrypoints, build/manifest, configuration, wiring/adapters, public APIs.",
		"Explicitly mention external nodes/services (APIs, queues, DBs, third-party SaaS).",
	},
	Assumptions: []string{
		"If uncertain, add to architecture_hypothesis.assumptions and reduce confidence.",
		"Unknowns belong in architecture_hypothesis.unknowns.",
	},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent(), llmtool.PresetCautious())

type M1 struct{ LLM llmclient.LLMClient }

// Run now accepts a single M1In to mirror M1's API.
func (p *M1) Run(ctx context.Context, in ml.M1In) (ml.M1Out, error) {
	if len(in.FileIndex) == 0 || len(in.MDDocs) == 0 {
		idx, mds := scanForM1(in.Repo, in.IgnoreDirs)
		if len(in.FileIndex) == 0 {
			in.FileIndex = idx
		}
		if len(in.MDDocs) == 0 {
			in.MDDocs = mds
		}
	}
	hints := in.Hints
	if hints == nil {
		hints = &ml.M1Hints{}
	}
	limits := in.Limits
	if limits == nil {
		limits = &ml.M1Limits{MaxNext: 8}
	}
	input := map[string]any{
		"file_index": in.FileIndex,
		"md_docs":    in.MDDocs,
		"hints":      hints,
		"limits":     map[string]any{"max_next": limits.MaxNext},
	}
	prompt, err := llmtool.StructuredPromptBuilder(m1PromptSpec)(ctx, &llmtool.ToolState{Input: input}, nil)
	if err != nil {
		return ml.M1Out{}, err
	}
	raw, err := p.LLM.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return ml.M1Out{}, err
	}
	var out ml.M1Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return ml.M1Out{}, fmt.Errorf("M1 JSON invalid: %w", err)
	}
	return out, nil
}

func scanForM1(repo string, ignore []string) ([]t.FileIndexEntry, []t.MDDoc) {
	var idx []t.FileIndexEntry
	var mds []t.MDDoc
	stripMD := regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	stripHTML := regexp.MustCompile(`(?is)<img[^>]*>`)
	_ = scan.ScanWithOptions(repo, scan.Options{IgnoreDirs: ignore}, func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		idx = append(idx, t.FileIndexEntry{Path: f.Path, Size: f.Size})
		if strings.EqualFold(f.Ext, ".md") {
			if b, e := scan.CurrentSafeFS().SafeReadFile(f.AbsPath); e == nil {
				txt := string(b)
				txt = stripMD.ReplaceAllString(txt, "")
				txt = stripHTML.ReplaceAllString(txt, "")
				mds = append(mds, t.MDDoc{Path: f.Path, Text: txt})
			}
		}
	})
	return idx, mds
}
