package pipeline

import (
    "context"
    "encoding/json"

    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P1 struct { LLM llm.LLMClient }

func (p *P1) Run(ctx context.Context, docSnips, entrySnips []any, manifests map[string]string) (t.P1Out, error) {
    prompt := `You are a senior software architect.
Using the documentation/code excerpts and manifests, produce an initial architecture hypothesis and choose files to read next.

Return STRICT JSON ONLY:
{
  "taxonomy": [{"kind":"system|subsystem|module|class|function|endpoint|schema|config","desc":"string"}],
  "parent_nodes": [{"id":"string","name":"string","kind":"system|subsystem|module","confidence":0.0,"provenance":["string"]}],
  "glossary": [{"term":"string","desc":"string","confidence":0.0}],
  "reading_policy": ["string","..."],
  "read_targets": [{"path":"string","reason":"string","confidence":0.0}]
}

Rules:
- Define taxonomy briefly; map high-level terms only.
- read_targets = 10–20 items max; prefer files with exports, routing, or public APIs.
- Use lower confidence when unclear; always include provenance.`

    input := map[string]any{"doc_snippets": docSnips, "entry_snippets": entrySnips, "manifests": manifests}
    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P1Out{}, err }
    var out t.P1Out; err = json.Unmarshal(raw, &out); return out, err
}
