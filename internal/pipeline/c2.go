package pipeline

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"insightify/internal/scan"
	t "insightify/internal/types"
)

const promptC2 = `You are Stage C2 of a static analysis pipeline.

Goal
- Given code snippets around import/include statements, clarify dependency relationships per file.
- Distinguish internal (repo) dependencies from external libraries.

Input (JSON)
{
  "snippets": [
    {"file_path": "src/a.ts", "start_line": 8, "end_line": 16, "code": "..."}
  ]
}

Task
- For each snippet's file, extract dependencies evident in the provided code.
- For each dependency, output:
  - source: raw module string as seen in the code (e.g., "./util", "react").
  - is_library: true when the dependency is an external package; false for internal repo usage.
  - resolved_path: repo-relative path when internal and reasonably inferable; omit if unknown.
  - symbols: identifiers imported/used for this source (e.g., ["Foo", "default"]).
  - lines: snippet line numbers that evidence this dependency.
  - reason: short phrase stating why you classified it that way.
- Prefer omission over guessing. Never invent missing files.

Heuristics
- Internal when path starts with "./" or "../" or clearly refers to a repo path.
- Common resolution: "foo" → "foo.ts|tsx|js|jsx|mjs|cjs" or "foo/index.ts|js|..." when obviously internal; otherwise omit.
- Barrel files (index.*) are allowed; still prefer a concrete file when inferable.
- Bare package names (e.g., "react", "lodash", "@scope/pkg") are external libraries.
- Framework-specific import styles are libraries unless clearly project-local.

Output (STRICT JSON; no comments; omit empty fields)
{
  "dependencies": [
    {
      "file": "src/a.ts",
      "imports": [
        {
          "source": "./util",
          "resolved_path": "src/util.ts",
          "is_library": false,
          "symbols": ["Foo"],
          "lines": [9],
          "reason": "relative import"
        }
      ]
    }
  ],
  "unresolved": [
    {"file": "src/a.ts", "source": "@alias/core", "why": "aliased path; insufficient info"}
  ]
}

Constraints
- JSON only; no trailing commas. Use forward slashes in paths.
- It is acceptable to return zero dependencies when none are evident.
- Input may be chunked; treat independently. Duplicates across chunks are fine.

Example
Input:
{"snippets":[{"file_path":"src/a.ts","start_line":8,"end_line":16,"code":"import {Foo} from './util'\nimport React from 'react'\n"}]}
Output:
{
  "dependencies": [
    {"file":"src/a.ts","imports":[
      {"source":"./util","resolved_path":"src/util.ts","is_library":false,"symbols":["Foo"],"lines":[8],"reason":"relative import"},
      {"source":"react","is_library":true,"symbols":["default"],"lines":[9],"reason":"bare package import"}
    ]}
  ]
}`

// C2 uses LLM to refine dependency relationships using code snippets.
type C2 struct{ LLM llmClient }

// small interface to avoid import loops
type llmClient interface {
	GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error)
}

func (x C2) Run(ctx context.Context, in t.C2In) (t.C2Out, error) {
	// Collect snippet ranges from input
	var all []scan.Snippet
	for _, grp := range in.Stmts {
		for _, r := range grp.StmtRange {
			s, err := scan.ReadSnippet(in.Repo, scan.SnippetInput{FilePath: r.FilePath, StartLine: r.StartLine, EndLine: r.EndLine})
			if err != nil {
				continue
			}
			all = append(all, s)
		}
	}

	chunks := scan.ChunkSnippets(all, 5_000)

	// Aggregate dependencies across chunks
	requiresByFile := map[string]map[string]struct{}{}

	type llmIn struct {
		Snippets []scan.Snippet `json:"snippets"`
	}
	type llmOut struct {
		Dependencies []struct {
			File     string   `json:"file"`
			Requires []string `json:"requires"`
		} `json:"dependencies"`
	}

	for _, ch := range chunks {
		inp := llmIn{Snippets: ch}
		raw, err := x.LLM.GenerateJSON(ctx, promptC2, inp)
		if err != nil {
			continue
		}
		var out llmOut
		if err := json.Unmarshal(raw, &out); err != nil {
			continue
		}
		for _, d := range out.Dependencies {
			f := filepath.ToSlash(strings.TrimSpace(d.File))
			if f == "" {
				continue
			}
			set := requiresByFile[f]
			if set == nil {
				set = map[string]struct{}{}
				requiresByFile[f] = set
			}
			for _, r := range d.Requires {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				set[r] = struct{}{}
			}
		}
	}

	// Build C2Out
	var out t.C2Out
	for f, set := range requiresByFile {
		var req []string
		for k := range set {
			req = append(req, k)
		}
		out.Dependencies = append(out.Dependencies, t.FileWithDependency{
			Path:     f,
			Requires: req,
		})
	}
	return out, nil
}
