package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P1 struct { LLM llm.LLMClient }

func (p *P1) Run(ctx context.Context, docSnips, entrySnips []any, manifests any) (t.P1Out, error) {
    prompt := `You are a senior software architect. Based on the provided documentation and code excerpts, build an initial architecture hypothesis and list important files to read next. Return strictly JSON with schema:
{
  "taxonomy":[{"kind":"string","desc":"string"}],
  "parent_nodes":[{"id":"string","name":"string","kind":"string","confidence":0.0}],
  "glossary":[{"term":"string","desc":"string","confidence":0.0}],
  "reading_policy":["string", "..."],
  "read_targets":[{"path":"string","reason":"string","confidence":0.0}]
}`
    input := map[string]any{"doc_snippets": docSnips, "entry_snippets": entrySnips, "manifests": manifests}
    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P1Out{}, err }
    var out t.P1Out; err = json.Unmarshal(raw, &out); return out, err
}
