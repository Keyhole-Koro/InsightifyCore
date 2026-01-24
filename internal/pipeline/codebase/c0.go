package codebase

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/scan"
)

// C0 prompt — imports/includes only, plus normalization hints for later post-processing.
var c0PromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Emit one spec per language family present in extension counts for dependency analysis.",
	Background:   "Stage C0 analyzes file extension counts to detect language families and generate heuristic rules for import extraction.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.C0Out{}),
	Constraints: []string{
		"Emit specs **only** for families that appear in 'ext_counts'.",
		"Use the real family key (e.g., 'js', 'py', 'go') — no placeholders.",
		"Keep lists concise ('keywords' ≤ ~8, 'path_split' ≤ ~6).",
		"Echo extensions exactly as seen (with leading dot).",
		"Produce **valid JSON**. No comments. No regex patterns. No fields other than those listed.",
		"If unknown, use empty arrays instead of inventing values.",
	},
	Rules: []string{
		"Group related extensions into a **single** spec when they are the same language family (e.g., JavaScript/TypeScript -> .js, .mjs, .cjs, .jsx, .ts, .tsx).",
		"Only include extensions that are actually present in 'ext_counts'.",
		"Every extension listed in 'spec.ext' is an **interchangeable** candidate for resolution.",
	},
	Assumptions:  []string{"Missing families should be ignored."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent())

type C0 struct{ LLM llmclient.LLMClient }

func (x *C0) Run(ctx context.Context, in artifact.C0In) (artifact.C0Out, error) {
	// Populate ext counts if missing so runner BuildInput can stay lightweight.
	if len(in.ExtCounts) == 0 {
		exts, err := computeExtCounts(ctx, in.Repo, in.Roots)
		if err != nil {
			return artifact.C0Out{}, err
		}
		in.ExtCounts = exts
	}

	// add the content of runtime config files to the input later
	input := map[string]any{
		"ext_counts": in.ExtCounts,
		"roots":      in.Roots, // Pass roots context for hints
	}

	prompt, err := llmtool.StructuredPromptBuilder(c0PromptSpec)(ctx, &llmtool.ToolState{Input: input}, nil)
	if err != nil {
		return artifact.C0Out{}, err
	}

	raw, err := x.LLM.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return artifact.C0Out{}, err
	}

	var out artifact.C0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.C0Out{}, fmt.Errorf("C0 JSON invalid: %w\nraw: %s", err, string(raw))
	}
	out.Families = out.Families[:0]
	for family, specKeys := range out.FamilyKeys {
		keys := append([]string(nil), specKeys...)
		sort.Strings(keys)
		for _, key := range keys {
			spec, ok := out.Specs[key]
			if !ok {
				continue
			}
			out.Families = append(out.Families, artifact.FamilySpec{
				Family: family,
				Key:    key,
				Spec:   spec,
			})
		}
	}
	sort.Slice(out.Families, func(i, j int) bool {
		if out.Families[i].Family == out.Families[j].Family {
			return out.Families[i].Key < out.Families[j].Key
		}
		return out.Families[i].Family < out.Families[j].Family
	})
	return out, nil
}

func computeExtCounts(ctx context.Context, repo string, roots artifact.M0Out) ([]artifact.ExtCount, error) {
	_ = ctx
	ignore := make(map[string]struct{})
	for _, r := range roots.LibraryRoots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		base := filepath.Base(filepath.ToSlash(r))
		if base == "" {
			continue
		}
		ignore[base] = struct{}{}
	}
	var ignoreDirs []string
	for k := range ignore {
		ignoreDirs = append(ignoreDirs, k)
	}

	extCountMap := map[string]int{}
	if err := scan.ScanWithOptions(repo, scan.Options{IgnoreDirs: ignoreDirs}, func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		ext := f.Ext
		if ext == "" {
			ext = filepath.Ext(f.Path)
		}
		if ext != "" {
			extCountMap[strings.ToLower(ext)]++
		}
	}); err != nil {
		return nil, err
	}

	extCounts := make([]artifact.ExtCount, 0, len(extCountMap))
	for ext, cnt := range extCountMap {
		extCounts = append(extCounts, artifact.ExtCount{Ext: ext, Count: cnt})
	}
	sort.Slice(extCounts, func(i, j int) bool {
		if extCounts[i].Count == extCounts[j].Count {
			return extCounts[i].Ext < extCounts[j].Ext
		}
		return extCounts[i].Count > extCounts[j].Count
	})
	return extCounts, nil
}
