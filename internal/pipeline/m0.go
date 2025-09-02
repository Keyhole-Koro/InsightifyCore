package pipeline

import (
    "context"
    "encoding/json"
    "fmt"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

const promptM0 = `You are ifying a repository layout.

Input JSON provides:
- ext_counts: map of file extensions to counts across the repo (e.g., ".ts": 120)
- dirs_depth1: top-level directories (one-segment paths)
- dirs_depth2: second-level directories (two-segment paths like "src/app")

Task:
Return STRICT JSON identifying likely roots for:
{
  "main_source_roots":   ["string"],  // primary application code dirs
  "library_roots":       ["string"],  // internal/shared libraries OR third-party vendor/dependency roots to be skipped in analysis (e.g., node_modules, vendor, third_party)
  "config_roots":        ["string"],  // configuration, infra, or ops (e.g., .github, config/, scripts/)
  "runtime_config_roots": ["string"], // paths that influence runtime behavior (env/templates/migrations/etc.)
  "notes":               ["string"]   // short rationale for each category
}

Rules:
- Use repo-relative directory paths using forward slashes.
- Prefer concrete subpaths at depth 1 or 2.
- If uncertain, keep the list small and add a note.
- JSON only; no comments or trailing commas.
- Treat large dependency/vendor directories as library_roots when present:
- prefer top-level roots only (e.g., "node_modules", not "node_modules/*").
- Common examples: "node_modules", "vendor", "third_party", ".venv", "venv".
- Keep lists small; do not explode subpackages under vendor directories.
`

type M0 struct{ LLM llm.LLMClient }

func (p *M0) Run(ctx context.Context, in t.M0In) (t.M0Out, error) {
    input := map[string]any{
        "ext_counts":  in.ExtCounts,
        "dirs_depth1": in.DirsDepth1,
        "dirs_depth2": in.DirsDepth2,
    }
    raw, err := p.LLM.GenerateJSON(ctx, promptM0, input)
    if err != nil {
        return t.M0Out{}, err
    }
    var out t.M0Out
    if err := json.Unmarshal(raw, &out); err != nil {
        return t.M0Out{}, fmt.Errorf("M0 JSON invalid: %w\nraw: %s", err, string(raw))
    }
    return out, nil
}
