package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    s "insightify/internal/scan"
    t "insightify/internal/types"
)

type P2 struct { LLM llm.LLMClient }

func (p *P2) Run(ctx context.Context, dir string, snips []s.Snippet, tax []t.KV, gloss []t.GlossEntry) (t.P2Out, error) {
    prompt := `You are a senior software architect.
Summarize the target directory based on code snippets and the shared glossary/taxonomy.

Return STRICT JSON ONLY:
{
  "dir": "string",
  "role": {"summary":"string","confidence":0.0,"provenance":["string"]},
  "public_api": [{"name":"string","kind":"class|function|endpoint|schema","path":"string","signature":"string","provenance":["string"]}],
  "identifiers": [{"name":"string","kind":"class|function|type|interface|const|var","path":"string","provenance":["string"]}],
  "notable_files": [{"path":"string","reason":"string","confidence":0.0}]
}

Constraints:
- Avoid global claims; stick to evidence in the snippets.
- Prefer exports, routing, controllers, service classes, schema/DTO definitions.
- public_api ≤ 20, identifiers ≤ 40, notable_files ≤ 8.`

    input := map[string]any{"dir": dir, "snippets": snips, "taxonomy": tax, "glossary": gloss}
    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P2Out{}, err }
    var out t.P2Out; err = json.Unmarshal(raw, &out); return out, err
}