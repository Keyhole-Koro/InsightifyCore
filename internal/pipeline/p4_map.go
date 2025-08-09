package pipeline

import (
	"context"

	"insightify/internal/util/jsonutil"
	"insightify/internal/llm"
	t "insightify/internal/types"
)

type P4Map struct{ LLM llm.LLMClient }

// Run: directory-local, abstract relations only (language-agnostic).
func (p *P4Map) Run(ctx context.Context, nodes []t.Node, ev t.P4Evidence) (t.P4Out, error) {
	prompt := `You are a senior software architect.
Use language-agnostic Signals to infer LOCAL abstract relations between nodes.

Edge types (use only these):
- contains, depends_on, invokes, exchanges, persists, configures, observes

STRICT JSON ONLY:
{
  "edges": [
    {"id":"string","source":"string","target":"string",
     "type":"contains|depends_on|invokes|exchanges|persists|configures|observes",
     "attrs":{"channel":"network|storage|message|process|unknown","sync":"sync|async|unknown","boundary":"internal|external|unknown","verb":"string"},
     "evidence":[{"file":"string","lines":[0,0]}],
     "confidence":0.0}
  ],
  "artifacts": {
    "interfaces": [{"name":"string","kind":"api|rpc|cli|event|other","where":"string","provenance":[{"file":"string","lines":[0,0]}]}],
    "schemas":    [{"name":"string","where":"string","provenance":[{"file":"string","lines":[0,0]}]}],
    "config":     [{"key":"string","where":"string","provenance":[{"file":"string","lines":[0,0]}]}]
  }
}

Inputs:
- nodes_index: list of {id, paths, identifiers}
- signals: array of {kind, file, range, attrs, text}
- dir: current directory

Mapping guidance (technology-agnostic):
- bind     -> depends_on (static)
- invoke   -> invokes (control-flow)
- io       -> exchanges (data-flow). Infer attrs.channel from hints (URL-ish => network, SQL-ish => storage, queue-ish => message), else "unknown".
- declare  -> may inform contains when file path matches a node (module owns class).
- annotate -> may adjust boundary (external) or confidence.
- file_head -> context only; must not alone create edges.

Resolution:
- Resolve source/target by LONGEST match of file path under nodes_index.paths; if multiple match, choose the most specific and lower confidence.
- Edge ID: "edge:<type>:<source>-><target>".
- Merge duplicates within this batch by concatenating evidence.
- No narrative text—JSON only.`

	// Slim index to save tokens
	var nodesIndex []map[string]any
	for _, n := range nodes {
		nodesIndex = append(nodesIndex, map[string]any{
			"id":          n.ID,
			"paths":       n.Paths,
			"identifiers": n.Identifiers,
		})
	}

	input := map[string]any{
		"nodes_index": nodesIndex,
		"signals":     ev.Signals,
		"dir":         ev.Dir,
	}

	raw, err := p.LLM.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return t.P4Out{}, err
	}
	var out t.P4Out
	if err := jsonutil.Unmarshal(raw, &out); err != nil {
		return t.P4Out{}, err
	}
	return out, nil
}
