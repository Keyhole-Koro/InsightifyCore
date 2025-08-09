package pipeline

import (
    "context"

	"insightify/internal/util/jsonutil"
    "insightify/internal/llm"
    t "insightify/internal/types"
)

type P3 struct { LLM llm.LLMClient }

func (p *P3) Run(ctx context.Context, p2Summaries []t.P2Out, glossary []t.GlossEntry, taxonomy []t.KV) (t.P3Out, error) {
	prompt := `You are a senior software architect.
From directory-level summaries (P2), a glossary, and a taxonomy, propose a SMALL, stable set of nodes.
Focus on ABSTRACT nodes (system/subsystem/module). Emit CODE-backed nodes only when the evidence is explicit.

STRICT JSON ONLY with this schema:
{
  "nodes": [
    {
      "id": "node:<kind>:<slug>",
      "name": "string",
      "kind": "system|subsystem|module|class|schema|config",
      "layer": 0,
      "origin": "abstract|code",
      "paths": ["glob"],
      "span": [{"file":"string","lines":[0,0]}],
      "identifiers": ["string"],
      "confidence": 0.0,
      "provenance": [{"file":"string","lines":[0,0]}]
    }
  ],
  "open_questions": ["string"]
}

Layer policy (MUST honor):
- system=0, subsystem=1, module=2, class|schema|config=3, function=4  (do NOT emit function here).

Rules:
- Use stable IDs: node:<kind>:<slug-from-path-or-name> (lower-kebab-case).
- Prefer ABSTRACT nodes at layers 0-2; only add code-backed nodes when P2 evidence is explicit.
- Set origin="abstract" unless clearly code-backed (then origin="code" and include span/identifiers).
- paths should be directory globs like "src/**" derived from P2 dir; do not invent non-existent paths.
- Do NOT include fields other than those in the schema (no endpoints/protocols/interfaces/embedding).
- Deduplicate by responsibility; merge near-duplicates and keep one name.
- Lower confidence if uncertain and add a note to open_questions.
- Never output narrative text—JSON only.`

    input := map[string]any{
        "p2_summaries": p2Summaries,
        "glossary": glossary,
        "taxonomy": taxonomy,
    }

    raw, err := p.LLM.GenerateJSON(ctx, prompt, input); if err != nil { return t.P3Out{}, err }
    var out t.P3Out; err = jsonutil.Unmarshal(raw, &out); return out, err
}