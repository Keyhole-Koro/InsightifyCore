package mainline

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/scan"
)

var m0PromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Classify repository layout from extension counts and a shallow directory scan.",
	Background:   "Phase M0 identifies primary source roots, config locations, and runtime-impacting files to guide later analysis.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.M0Out{}),
	Constraints: []string{
		"Maintain the field order shown in OUTPUT.",
		"Use absolute (full) paths with forward slashes for both directories and files.",
		"config_files, runtime_config_files, and runtime_configs.path must be concrete file paths.",
		"Prefer depth-1 or depth-2 subpaths; avoid deep descendants unless unavoidable.",
		"Treat vendor/dependency dirs as library_roots when present (node_modules, vendor, third_party, .venv, venv).",
		"Keep lists concise; do not enumerate every child of large vendor directories.",
		"runtime_configs.ext must include the leading dot or be empty when there is no extension.",
	},
	Rules: []string{
		"If unsure, keep lists small and explain uncertainty in notes.",
	},
	Assumptions:  []string{"Missing categories can be empty arrays."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent(), llmtool.PresetCautious())

type M0 struct{ LLM llmclient.LLMClient }

func (p *M0) Run(ctx context.Context, in artifact.M0In) (artifact.M0Out, error) {
	if len(in.ExtCounts) == 0 || len(in.DirsDepth1) == 0 {
		exts, dirs := scanDepth1(in.Repo)
		if len(in.ExtCounts) == 0 {
			in.ExtCounts = exts
		}
		if len(in.DirsDepth1) == 0 {
			in.DirsDepth1 = dirs
		}
	}
	input := map[string]any{
		"ext_counts":  in.ExtCounts,
		"dirs_depth1": in.DirsDepth1,
	}
	prompt, err := llmtool.StructuredPromptBuilder(m0PromptSpec)(ctx, &llmtool.ToolState{Input: input}, nil)
	if err != nil {
		return artifact.M0Out{}, err
	}
	raw, err := p.LLM.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return artifact.M0Out{}, err
	}
	var out artifact.M0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.M0Out{}, fmt.Errorf("M0 JSON invalid: %w\nraw: %s", err, string(raw))
	}
	return out, nil
}

func scanDepth1(repo string) (map[string]int, []string) {
	extCounts := map[string]int{}
	var idx []string
	_ = scan.ScanWithOptions(repo, scan.Options{MaxDepth: 1}, func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		extCounts[f.Ext]++
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		if dir != "" && dir != "." {
			idx = append(idx, dir)
		}
	})
	dirSet := map[string]struct{}{}
	for _, d := range idx {
		dirSet[d] = struct{}{}
	}
	var dirs []string
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	return extCounts, dirs
}