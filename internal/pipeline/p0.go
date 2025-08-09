package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P0 struct { LLM llm.LLMClient }

func (p *P0) Run(ctx context.Context, tree any, heads map[string][]string, manifests map[string]string, entries []t.DocRef) (t.P0Out, error) {
    prompt := `You are a senior software architect.
From the provided repository tree summary, document headings, and manifests, propose the first documents to read and likely entry points.

Return STRICT JSON ONLY:
{
  "top_docs": [{"path":"string","reason":"string","confidence":0.0}],
  "entry_points": [{"path":"string","reason":"string","confidence":0.0}],
  "glossary_seed": [{"term":"string","desc":"string","confidence":0.0}],
  "next_actions": ["string","..."]
}

Constraints:
- Exclude build/vendor/node_modules/dist.
- Prefer README/architecture docs and manifest-referenced docs.
- Keep top_docs 5–12 items; entry_points 3–10 items.`

    input := map[string]any{"repo_tree": tree, "doc_heads": heads, "manifests": manifests, "entry_points": entries}
    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P0Out{}, err }
    var out t.P0Out; err = json.Unmarshal(raw, &out); return out, err
}