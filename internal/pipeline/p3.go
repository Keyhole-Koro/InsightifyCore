package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P3 struct { LLM llm.LLMClient }

func (p *P3) Run(ctx context.Context, p2Summaries []t.P2Out, glossary []t.GlossEntry, taxonomy []t.KV) (t.P3Out, error) {
    prompt := `You are a senior software architect.
Given a list of summarized directory roles, glossary definitions, and a taxonomy of system abstraction layers, generate candidate nodes representing architectural elements.

Return only STRICT JSON with the following schema:
{
  "nodes": [
    {
      "id": "string",
      "name": "string",
      "kind": "string",
      "layer": 0,
      "paths": ["string"],
      "span": [{"file": "string", "lines": [0, 0]}],
      "identifiers": ["string"],
      "interfaces": ["string"],
      "endpoints": ["string"],
      "protocols": ["string"],
      "embedding_hint": ["string"],
      "confidence": 0.0,
      "provenance": [{"file": "string", "lines": [0, 0]}]
    }
  ],
  "open_questions": ["string"]
}
Guidelines:
- Map kind→layer using provided taxonomy.
- Lower confidence if unsure. Always include provenance.`

    input := map[string]any{
        "p2_summaries": p2Summaries,
        "glossary": glossary,
        "taxonomy": taxonomy,
    }

    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P3Out{}, err }
    var out t.P3Out; err = json.Unmarshal(raw, &out); return out, err
}