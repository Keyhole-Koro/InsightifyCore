package mainline

import (
	"context"
	"encoding/json"
	"fmt"

	llmclient "insightify/internal/llmClient"
	ml "insightify/internal/types/mainline"
)

const prologue = `You are an experienced software engineer analyzing an unfamiliar codebase.

Purpose of the output:
- Provide a clear picture of what the system does and how it is structured so a reader can quickly understand the architecture.
- Explicitly mention external nodes/services (APIs, queues, DBs, third-party SaaS) that the system integrates with.

Approach:
- Identify relevant code → cite exact files/symbols → avoid guessing; if unknown, state it.

Common Rules & Constraints:
- Use repository-relative paths exactly as provided; never invent paths or filenames.
- Prefer code over docs when they disagree; report contradictions explicitly.
- Be technology-agnostic: do not assume frameworks, stacks, or runtimes unless observed in code or docs. If you infer, mark it as an assumption with low confidence.
- Cite evidence with {path, lines:[start,end]} using 1-based inclusive line numbers. If lines are unknown or unavailable, set lines to null and explain.
- Return ONLY valid JSON that matches the requested schema. No markdown, no commentary, no trailing commas.
- If inputs are incomplete, list what else you need under "needs_input" with exact filenames or glob patterns.
- When inputs are large, work incrementally: entrypoints → build/manifest → configuration → wiring/adapters → public APIs.
- Do not leak or reuse knowledge outside of the provided inputs.
- Keep names and paths case-sensitive.`

const promptM1 = prologue + `

Task:
From the file index and Markdown text (images/binaries excluded), construct an initial architecture hypothesis. Then propose the next files/patterns to open to confirm or refute that hypothesis.

Output: STRICT JSON with this schema (no extra fields):
{
  "architecture_hypothesis": {
    "purpose": "string",                          // What the system does and the big picture, including external nodes/services
    "summary": "string",
    "key_components": [
      {
        "name": "string",
        "kind": "string",
        "responsibility": "string",
        "evidence": [{"path":"string","lines":[1,2] | null}]
      }
    ],
    "execution_model": "string",
    "tech_stack": {
      "platforms": ["string"],
      "languages": ["string"],
      "build_tools": ["string"]
    },
    "assumptions": ["string"],
    "unknowns": ["string"],
    "confidence": 0.0
  },
  "next_files": [
    {"path":"string","reason":"string","what_to_check":["string"],"priority":1}
  ],
  "next_patterns": [
    {"pattern":"string","reason":"string","what_to_check":["string"],"priority":2}
  ],
  "contradictions": [
    {"claim":"string",
     "supports":[{"path":"string","lines":[1,2]|null}],
     "conflicts":[{"path":"string","lines":[1,2]|null}],
     "note":"string"}
  ],
  "needs_input": ["string"],
  "stop_when": ["string"],
  "notes": ["string"]
}

Constraints:
- Do NOT choose from fixed lists; use free-form tokens based on evidence. Use "unknown" only when genuinely unknown.
- Propose at most limits.max_next (default 8) across next_files + next_patterns.
- Evidence must reference provided paths; if you cannot identify lines, set lines to null and explain in notes.`

type M1 struct{ LLM llmclient.LLMClient }

// Run now accepts a single M1In to mirror M1's API.
func (p *M1) Run(ctx context.Context, in ml.M1In) (ml.M1Out, error) {
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
	raw, err := p.LLM.GenerateJSON(ctx, promptM1, input)
	if err != nil {
		return ml.M1Out{}, err
	}
	var out ml.M1Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return ml.M1Out{}, fmt.Errorf("M1 JSON invalid: %w", err)
	}
	return out, nil
}
