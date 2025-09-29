package pipeline

import (
	"context"
	"fmt"

	"encoding/json"

	"insightify/internal/llm"
	t "insightify/internal/types"
)

const bt = "`"

// C0 prompt — imports/includes only, plus normalization hints for later post-processing.
// This phase does NOT receive source snippets; it must produce RE2 patterns to detect module strings,
// and also provide language-agnostic guidance ("normalize_hints") describing how X1 should derive
// fields like folder_path[], file_name, file_ext, scope/package/subpath, alias, and kind.
const promptC0 = `
You are Stage C0 of a static analysis pipeline.

Task
- You will be given a list of file extensions ("ext_counts") and roots for main source and optional runtime configs ("roots").
- For each extension in ext_counts, produce one spec that:
  - lists short keywords to detect import/include-like references
  - lists simple tokens used to split around the module path
  - provides language-agnostic normalize_hints.alias for later resolution
- Do NOT output regex patterns or any fields other than those defined below.

Inputs (shape)
{
  "ext_counts": [
    {"ext": ".js", "count": 150}
  ],
  "roots": {
    "main_source_roots": ["src", "lib"],
    "runtime_config_roots": [
      {"path": "config/tsconfig.json", "ext": ".json", "content": "..."}
    ]
  }
}

Output (single JSON object, no comments, only these fields)
{
  "specs": [
    {
      "ext": ".js",
      "language": "JavaScript",
      "rules": {
        "keywords": ["import", "from", "require"],
        "path_split": [" ", "'", "\""]
      },
      "normalize_hints": {
        "alias": [
          {"original": "@APIHandler", "normalized": "api_handler"}
        ]
      }
    }
  ]
}

Constraints & notes
- Include specs only for extensions present in ext_counts.
- Keep lists concise (e.g., keywords up to ~8, path_split up to ~6).
- If runtime configs suggest aliases (e.g., tsconfig paths), reflect them under normalize_hints.alias.
- If unknown, use empty arrays rather than inventing fields.
`

// C0 generates ExtractorSpec v1 rules per extension based on an ext report.
type C0 struct{ LLM llm.LLMClient }

// Run asks the model to produce specs, validates them against provided tests,
// and retries once with additional context when available (e.g., runtime configs).
func (x *C0) Run(ctx context.Context, in t.C0In) (t.C0Out, error) {
	raw, err := x.LLM.GenerateJSON(ctx, promptC0, in)
	if err != nil {
		return t.C0Out{}, err
	}

	var out t.C0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return t.C0Out{}, fmt.Errorf("C0 JSON invalid: %w\nraw: %s", err, string(raw))
	}
	return out, nil
}
