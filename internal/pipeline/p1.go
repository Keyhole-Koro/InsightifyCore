package pipeline

import (
	"context"
	"fmt"
    "encoding/json"

	"insightify/internal/llm"
	t "insightify/internal/types"
)

const promptP1 = prologue + `

Task:
Read the newly provided files to confirm or refute the previous hypothesis. Update the hypothesis, record deltas, and propose the next most informative files or patterns.

Output: STRICT JSON with this schema (no extra fields):
{
  "updated_hypothesis": {
    "summary": "string",
    "key_components": [
      {"name":"string","kind":"string","responsibility":"string",
       "evidence":[{"path":"string","lines":[1,2]|null}]}
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
  "question_status": [
    {"path":"string","question":"string",
     "status":"confirmed|refuted|inconclusive",
     "evidence":[{"path":"string","lines":[1,2]|null}],
     "note":"string"}
  ],
  "delta": {
    "added": ["string"],
    "removed": ["string"],
    "modified": [{"field":"string","before": any,"after": any}]
  },
  "contradictions": [
    {"claim":"string",
     "supports":[{"path":"string","lines":[1,2]|null}],
     "conflicts":[{"path":"string","lines":[1,2]|null}],
     "resolution_hint":"string"}
  ],
  "next_files": [
    {"path":"string","reason":"string","what_to_check":["string"],"priority":1}
  ],
  "next_patterns": [
    {"pattern":"string","reason":"string","what_to_check":["string"],"priority":2}
  ],
  "needs_input": ["string"],
  "stop_when": ["string"],
  "notes": ["string"]
}

Constraints:
- No fixed vocabularies: report exactly what you observe or carefully infer. Lower confidence for inferences.
- Status each focus question; if inconclusive, specify exactly what else is needed.
- Propose at most limit_max_next (default 8) items across next_files + next_patterns.`

type P1 struct{ LLM llm.LLMClient }

func (p *P1) Run(ctx context.Context, in t.P1In) (t.P1Out, error) {
	input := map[string]any{
		"previous":      in.Previous,
		"opened_files":  in.OpenedFiles,
		"focus":         in.Focus,
		"file_index":    in.FileIndex,
		"md_docs":       in.MDDocs,
		"limit_max_next": in.LimitMaxNext,
	}
	raw, err := p.LLM.GenerateJSON(ctx, promptP1, input)
	if err != nil {
		return t.P1Out{}, err
	}

    fmt.Println("P1 raw output:", string(raw)) // Debugging line to see raw output
	var out t.P1Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return t.P1Out{}, fmt.Errorf("P1 JSON invalid: %w", err)
	}
	return out, nil
}
