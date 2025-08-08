package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P0 struct { LLM llm.LLMClient }

func (p *P0) Run(ctx context.Context, tree any, heads any, manifests any, entries []t.DocRef) (t.P0Out, error) {
    prompt := `You are a senior software architect. From the provided repository tree, document headings, manifests and entrypoint candidates, select the documents to read first. Return **ONLY** JSON with schema:
{
  "top_docs":[{"path":"string","reason":"string","confidence":0.0}],
  "entry_points":[{"path":"string","reason":"string","confidence":0.0}],
  "glossary_seed":[{"term":"string","desc":"string","confidence":0.0}],
  "next_actions":["string", "..."]
}`
    input := map[string]any{"repo_tree": tree, "doc_heads": heads, "manifests": manifests, "entry_points": entries}
    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P0Out{}, err }
    var out t.P0Out; err = json.Unmarshal(raw, &out); return out, err
}
