package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"insightify/internal/llm"
	t "insightify/internal/types"
)

// Preamble is assumed to be defined elsewhere as `prologue`. We rely on it.
// The schema below adds `updated_hypothesis.verification_targets`.
const promptM1 = prologue + `

You MUST output STRICT JSON that exactly matches the schema below. 
No comments, no trailing commas, no ellipses “…”, no backticks. 
If something is unknown, return null or an empty array/string explicitly. 
Paths must be repository-relative. Evidence line numbers are 1-based and inclusive; 
if unknown, set "lines": null (do NOT guess). 
Do not invent files, symbols, or line ranges.

Schema:
{
  "updated_hypothesis": {
    "purpose": "string",                          // What the system does and overall architecture, including external nodes/services
    "summary": "string",                          // One-paragraph hypothesis
    "key_components": [                           // Major parts you see or infer
      {
        "name": "string",
        "kind": "string",                         // Free-form (no fixed vocab)
        "responsibility": "string",
        "evidence": [ {"path":"string","lines":[1,2] | null} ],
        "likely_dependent_paths": ["string"]      // Folders/files that likely depend on this component
      }
    ],
    "execution_model": "string",                  // Free-form description
    "tech_stack": {
      "platforms": ["string"],                    // Free-form (e.g., "AWS API Gateway")
      "languages": ["string"],                    // Free-form (e.g., "TypeScript")
      "build_tools": ["string"]                   // Free-form
    },
    "assumptions": ["string"],                    // Clearly label inferences
    "unknowns": ["string"],                       // List open unknowns
    "confidence": 0.0,                            // 0.0 - 1.0
    "verification_targets": [                     // Direct verification targets for this hypothesis
      {"kind":"file|pattern|dir","path":"string","reason":"string","what_to_check":["string"],"coverage":"high|medium|low","priority":1}
    ]
  },

  "question_status": [                            // Map each Focus question -> status
    {
      "path": "string",                           // The main file you used (or intend) for this question
      "question": "string",                       // The focus item verbatim or normalized
      "status": "confirmed|refuted|inconclusive",
      "evidence": [ {"path":"string","lines":[1,2] | null} ],
      "note": "string"
    }
  ],

  "delta": {                                      // Changes vs previous hypothesis
    "added":   ["string"],
    "removed": ["string"],
    "modified": [
      {
        "field": "string",                        // e.g., "updated_hypothesis.summary" or "updated_hypothesis.key_components[2].responsibility"
        "before": any,                            // Type MUST match the target field's natural type; if unsure, use a string
        "after":  any
      }
    ]
  },

  "contradictions": [                             // Explicit conflicts you found
    {
      "claim": "string",
      "supports":  [ {"path":"string","lines":[1,2] | null} ],
      "conflicts": [ {"path":"string","lines":[1,2] | null} ],
      "resolution_hint": "string"
    }
  ],

  "next_files": [                                  // Concrete files to open next
    {"path":"string","reason":"string","what_to_check":["string"],"priority":1}
  ],
  "next_patterns": [                               // Globs/regex to expand search surface
    {"pattern":"string","reason":"string","what_to_check":["string"],"priority":2}
  ],

  "needs_input": ["string"],                       // Questions for the human
  "stop_when":  ["string"],                        // Convergence criteria
  "notes":      ["string"]                         // Misc short notes
}

Rules & Guidance:
- No fixed vocabularies: use precise, observed wording. Do NOT normalize to preset enums.
- Evidence: prefer exact line ranges; if you only know the file proves the point but cannot quote lines reliably, set "lines": null.
- Statusing: Every focus question MUST appear in "question_status". 
  - confirmed: direct evidence supports it; cite file+lines.
  - refuted: direct evidence contradicts it; cite file+lines.
  - inconclusive: not enough evidence in the opened files; propose where to look next.
- Verification vs. Next: 
  - "updated_hypothesis.verification_targets" = files/patterns that would directly verify statements INSIDE the current hypothesis.
  - "next_files"/"next_patterns" = the most informative items to open in the NEXT iteration (limit total across both to ≤ limit_max_next).
- For each key component, populate "likely_dependent_paths" with repository-relative folders/files that likely depend on, import, or call into that component. Prefer concrete subfolders (e.g., "src/routes/", "internal/handlers/") when evident.
- Delta.modified.field: use dotted/Indexed paths (e.g., "updated_hypothesis.key_components[1].evidence[0]").
- JSON only. Do not include Markdown, code fences, or prose outside the JSON object.

`

type M1 struct{ LLM llm.LLMClient }

// Run executes M1 with robust JSON handling and normalization.
func (p *M1) Run(ctx context.Context, in t.M1In) (t.M1Out, error) {
	input := map[string]any{
		"previous":        in.Previous,
		"opened_files":    in.OpenedFiles,
		"focus":           in.Focus,
		"file_index":      in.FileIndex,
		"md_docs":         in.MDDocs,
		"limit_max_next":  in.LimitMaxNext,
	}

    raw, err := p.LLM.GenerateJSON(ctx, promptM1, input)
    if err != nil {
        return t.M1Out{}, err
    }
    fmt.Printf("M1 raw output (%d bytes)\n", len(raw))

    var out t.M1Out
    if err := json.Unmarshal(raw, &out); err == nil {
        return out, nil
    }

	// Normalize known quirks and retry.
    norm, nerr := normalizeM1JSON(raw)
    if nerr != nil {
        return t.M1Out{}, fmt.Errorf("M1 JSON invalid and normalization failed: %w", nerr)
    }
    if err := json.Unmarshal(norm, &out); err != nil {
        return t.M1Out{}, fmt.Errorf("M1 JSON invalid after normalization: %w\npayload: %s", err, string(norm))
    }
    return out, nil
}

// normalizeM1JSON coerces known-quirk fields into a stable shape expected by t.M1Out:
// - delta.modified.before/after: stringify if array/object/number/bool
// - ensure arrays exist if omitted
// - ensure verification_targets exists as array under updated_hypothesis
func normalizeM1JSON(raw []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}

	// delta.modified.* to strings
	if delta, ok := m["delta"].(map[string]any); ok {
		if mods, ok := delta["modified"].([]any); ok {
			for i, it := range mods {
				obj, _ := it.(map[string]any)
				if obj == nil {
					continue
				}
				for _, k := range []string{"before", "after"} {
					v, exists := obj[k]
					if !exists || v == nil {
						continue
					}
					if _, isStr := v.(string); !isStr {
						b, _ := json.Marshal(v)
						obj[k] = string(b)
					}
				}
				mods[i] = obj
			}
			delta["modified"] = mods
			m["delta"] = delta
		}
	}

	// Ensure arrays exist for downstream stability.
	ensureArray := func(key string) {
		if _, ok := m[key]; !ok {
			m[key] = []any{}
		}
	}
	ensureArray("next_files")
	ensureArray("next_patterns")
	ensureArray("needs_input")
	ensureArray("stop_when")
	ensureArray("notes")
	ensureArray("question_status")
	ensureArray("contradictions")

	// Ensure updated_hypothesis.verification_targets is an array.
	if uh, ok := m["updated_hypothesis"].(map[string]any); ok {
		if _, ok := uh["verification_targets"]; !ok {
			uh["verification_targets"] = []any{}
		}
		m["updated_hypothesis"] = uh
	}

	// Re-encode without HTML escaping.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}
