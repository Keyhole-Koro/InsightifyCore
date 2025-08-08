package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P3 struct { LLM llm.LLMClient }

func (p *P3) Run(ctx context.Context, p2Summaries []t.P2Out, glossary []t.GlossEntry, taxonomy []t.KV) (t.P3Out, error) {
    prompt := `You are a software architect. Given a list of summarized directory roles, glossary definitions, and system taxonomy, identify semantic or functional links between directories/components.

Return only JSON with the following schema:
{
  "links": [
    {"from": "string", "to": "string", "kind": "string", "confidence": 0.0, "provenance": ["string"]}
  ],
  "notes": ["string", "..."]
}`

    input := map[string]any{
        "p2_summaries": p2Summaries,
        "glossary": glossary,
        "taxonomy": taxonomy,
    }

    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P3Out{}, err }
    var out t.P3Out; err = json.Unmarshal(raw, &out); return out, err
}
