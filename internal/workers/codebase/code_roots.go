package codebase

import (
	"context"
	"encoding/json"
	"fmt"
	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	"insightify/internal/scan"
	"insightify/internal/utils"
	"path/filepath"
)

var codeRootsPromptSpec = llmtool.ApplyPresets(llmtool.StructuredPromptSpec{
	Purpose:      "Classify repository layout from extension counts and a shallow directory scan.",
	Background:   "Worker CodeRoots identifies primary source roots, config locations, and runtime-impacting files to guide later analysis.",
	OutputFields: llmtool.MustFieldsFromStruct(artifact.CodeRootsOut{}),
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
		"You may use the 'scan.list' tool to inspect specific subdirectories if the initial scan is insufficient.",
	},
	Assumptions:  []string{"Missing categories can be empty arrays."},
	OutputFormat: "JSON only.",
	Language:     "English",
}, llmtool.PresetStrictJSON(), llmtool.PresetNoInvent(), llmtool.PresetCautious())

type CodeRoots struct {
	LLM   llmclient.LLMClient
	Tools llmtool.ToolProvider
}

func (p *CodeRoots) Run(ctx context.Context, in artifact.CodeRootsIn) (artifact.CodeRootsOut, error) {
	if len(in.ExtCounts) == 0 || len(in.Dirs) == 0 {
		exts, dirs := scanRepoLayout(in.Repo)
		if len(in.ExtCounts) == 0 {
			in.ExtCounts = exts
		}
		if len(in.Dirs) == 0 {
			in.Dirs = dirs
		}
	}

	input := map[string]any{
		"ext_counts": in.ExtCounts,
		"dir_tree":   utils.PathsToTree(in.Dirs),
	}

	loop := &llmtool.ToolLoop{
		LLM:      p.LLM,
		Tools:    p.Tools,
		MaxIters: 5,
		Allowed:  []string{"scan.list"},
	}

	raw, _, err := loop.Run(ctx, input, llmtool.StructuredPromptBuilder(codeRootsPromptSpec))
	if err != nil {
		return artifact.CodeRootsOut{}, err
	}
	var out artifact.CodeRootsOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.CodeRootsOut{}, fmt.Errorf("CodeRoots JSON invalid: %w\nraw: %s", err, string(raw))
	}
	return out, nil
}

func scanRepoLayout(repo string) (map[string]int, []string) {
	extCounts := map[string]int{}
	var idx []string
	// Deeper scan to find nested configs/source, but limit per-dir noise.
	_ = scan.ScanWithOptions(repo, scan.Options{MaxDepth: 3, MaxPerDir: 10}, func(f scan.FileVisit) {
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
