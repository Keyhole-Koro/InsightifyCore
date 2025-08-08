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
    prompt := `You are a senior software architect. Summarize the given directory: purpose, public API, key identifiers, notable files. Return JSON following schema:
{
  "dir":"string",
  "role":{"summary":"string","confidence":0.0,"provenance":["string"]},
  "public_api":[{"name":"string","kind":"string","path":"string","signature":"string","provenance":["string"]}],
  "identifiers":[{"name":"string","kind":"string","path":"string","provenance":["string"]}],
  "notable_files":[{"path":"string","reason":"string","confidence":0.0}]
}`
    input := map[string]any{
        "dir": dir,
        "snippets": snips,
        "taxonomy": tax,
        "glossary": gloss,
    }
    
    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P2Out{}, err }
    var out t.P2Out; err = json.Unmarshal(raw, &out); return out, err
}