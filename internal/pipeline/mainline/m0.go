package mainline

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/scan"
	ml "insightify/internal/types/mainline"
)

const promptM0 = `You are classifying the layout of a repository.

Input JSON provides:
- ext_counts: map of file extensions to counts across the repo (e.g., ".ts": 120)
- dirs_depth1: repo-relative folder paths discovered during a shallow scan

Produce STRICT JSON with exactly the following fields (omit arrays when empty):
{
  "main_source_roots":     ["<dir>"],  // primary application code directories
  "library_roots":         ["<dir>"],  // shared libs or vendored deps to skip in analysis (e.g., node_modules, vendor)
  "config_roots":          ["<dir>"],  // configuration/infra/ops directories (e.g., .github, config/, scripts/)
  "runtime_config_roots":  ["<dir>"],  // directories whose files affect runtime behaviour (env/templates/migrations/etc.)
  "config_files":          ["<file>"], // specific config file paths (e.g., .env, docker-compose.yml)
  "runtime_config_files":  ["<file>"], // runtime-impacting file paths (e.g., env files, migrations, templates)
  "runtime_configs": [
    { "path": "<file>", "ext": "<dot-extension>" }
  ],
  "build_roots":          ["<dir>"],
  "notes":                ["<short rationale>"]
}

Rules:
- Use **absolute (full) paths** with forward slashes for both directories and files.
- Entries in config_files, runtime_config_files, and runtime_configs.path must be concrete file paths (include extension when present).
- Prefer depth-1 or depth-2 subpaths; avoid listing deep descendants unless unavoidable.
- If unsure, keep the list small and explain uncertainty in notes.
- JSON only; no comments or extra fields. Maintain the field order above.
- Treat large dependency/vendor directories as library_roots when present (e.g., node_modules, vendor, third_party, .venv, venv).
- Keep lists concise; do not enumerate every child of large vendor directories.
- For runtime_configs entries, omit "content" and any other fields not shown; include "ext" with the leading dot or "" if the file has no extension.
`

type M0 struct{ LLM llmclient.LLMClient }

func (p *M0) Run(ctx context.Context, in ml.M0In) (ml.M0Out, error) {
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
	raw, err := p.LLM.GenerateJSON(ctx, promptM0, input)
	if err != nil {
		return ml.M0Out{}, err
	}
	var out ml.M0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return ml.M0Out{}, fmt.Errorf("M0 JSON invalid: %w\nraw: %s", err, string(raw))
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
