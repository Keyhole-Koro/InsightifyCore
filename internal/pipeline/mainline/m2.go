package mainline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/scan"
	t "insightify/internal/types"
	ml "insightify/internal/types/mainline"
)

// Preamble is assumed to be defined elsewhere as `prologue`. We rely on it.
// The schema below adds `updated_hypothesis.verification_targets`.
const promptM2 = prologue + `

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

type M2 struct{ LLM llmclient.LLMClient }

// Run executes M2 with robust JSON handling and normalization.
func (p *M2) Run(ctx context.Context, in ml.M2In) (ml.M2Out, error) {
	if len(in.FileIndex) == 0 || len(in.MDDocs) == 0 || len(in.OpenedFiles) == 0 || len(in.Focus) == 0 {
		var m1 ml.M1Out
		if prev, ok := in.Previous.(ml.M1Out); ok {
			m1 = prev
		}
		var ignore []string
		var roots []string
		if in.Roots != nil {
			ignore = uniqueStrings(baseNames(in.Roots.LibraryRoots...))
			roots = in.Roots.MainSourceRoots
		}
		if len(in.OpenedFiles) == 0 || len(in.Focus) == 0 {
			opened, focus := buildOpenedAndFocus(m1, in.RepoRoot, in.LimitMaxNext)
			if len(in.OpenedFiles) == 0 {
				in.OpenedFiles = opened
			}
			if len(in.Focus) == 0 {
				in.Focus = focus
			}
		}
		if len(in.FileIndex) == 0 || len(in.MDDocs) == 0 {
			idx, mds := scanForM2(in.Repo, ignore)
			if len(roots) > 0 {
				idx = filterIndexByRoots(idx, roots)
				mds = filterMDDocsByRoots(mds, roots)
			}
			if len(in.FileIndex) == 0 {
				in.FileIndex = idx
			}
			if len(in.MDDocs) == 0 {
				in.MDDocs = mds
			}
		}
	}
	input := map[string]any{
		"previous":       in.Previous,
		"opened_files":   in.OpenedFiles,
		"focus":          in.Focus,
		"file_index":     in.FileIndex,
		"md_docs":        in.MDDocs,
		"limit_max_next": in.LimitMaxNext,
	}

	raw, err := p.LLM.GenerateJSON(ctx, promptM2, input)
	if err != nil {
		return ml.M2Out{}, err
	}
	fmt.Printf("M2 raw output (%d bytes)\n", len(raw))

	var out ml.M2Out
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}

	// Normalize known quirks and retry.
	norm, nerr := normalizeM2JSON(raw)
	if nerr != nil {
		return ml.M2Out{}, fmt.Errorf("M2 JSON invalid and normalization failed: %w", nerr)
	}
	if err := json.Unmarshal(norm, &out); err != nil {
		return ml.M2Out{}, fmt.Errorf("M2 JSON invalid after normalization: %w\npayload: %s", err, string(norm))
	}
	return out, nil
}

// normalizeM2JSON coerces known-quirk fields into a stable shape expected by t.M2Out:
// - delta.modified.before/after: stringify if array/object/number/bool
// - ensure arrays exist if omitted
// - ensure verification_targets exists as array under updated_hypothesis
func normalizeM2JSON(raw []byte) ([]byte, error) {
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

func buildOpenedAndFocus(m1 ml.M1Out, repoRoot string, limit int) ([]t.OpenedFile, []t.FocusQuestion) {
	var opened []t.OpenedFile
	var focus []t.FocusQuestion
	if limit <= 0 {
		limit = 8
	}
	fs := scan.CurrentSafeFS()
	picked := 0
	for _, nf := range m1.NextFiles {
		if picked >= limit {
			break
		}
		full := filepath.Join(repoRoot, filepath.Clean(nf.Path))
		b, err := fs.SafeReadFile(full)
		if err != nil {
			continue
		}
		opened = append(opened, t.OpenedFile{Path: nf.Path, Content: string(b)})
		if len(nf.WhatToCheck) == 0 {
			focus = append(focus, t.FocusQuestion{Path: nf.Path, Question: "Review this file for key architecture details"})
		} else {
			for _, q := range nf.WhatToCheck {
				focus = append(focus, t.FocusQuestion{Path: nf.Path, Question: q})
			}
		}
		picked++
	}
	return opened, focus
}

func scanForM2(repo string, ignore []string) ([]t.FileIndexEntry, []t.MDDoc) {
	var index []t.FileIndexEntry
	var mdDocs []t.MDDoc
	stripMD := regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	stripHTML := regexp.MustCompile(`(?is)<img[^>]*>`)
	_ = scan.ScanWithOptions(repo, scan.Options{IgnoreDirs: ignore}, func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		index = append(index, t.FileIndexEntry{Path: f.Path, Size: f.Size})
		if strings.EqualFold(f.Ext, ".md") {
			if b, e := scan.CurrentSafeFS().SafeReadFile(f.AbsPath); e == nil {
				txt := string(b)
				txt = stripMD.ReplaceAllString(txt, "")
				txt = stripHTML.ReplaceAllString(txt, "")
				mdDocs = append(mdDocs, t.MDDoc{Path: f.Path, Text: txt})
			}
		}
	})
	return index, mdDocs
}

func filterIndexByRoots(index []t.FileIndexEntry, roots []string) []t.FileIndexEntry {
	if len(roots) == 0 {
		return index
	}
	var out []t.FileIndexEntry
	for _, it := range index {
		for _, r := range roots {
			r = strings.Trim(strings.TrimSpace(r), "/")
			if r == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(it.Path), strings.ToLower(r+"/")) || strings.EqualFold(it.Path, r) {
				out = append(out, it)
				break
			}
		}
	}
	return out
}

func filterMDDocsByRoots(docs []t.MDDoc, roots []string) []t.MDDoc {
	if len(roots) == 0 {
		return docs
	}
	var out []t.MDDoc
	for _, d := range docs {
		for _, r := range roots {
			r = strings.Trim(strings.TrimSpace(r), "/")
			if r == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(d.Path), strings.ToLower(r+"/")) || strings.EqualFold(d.Path, r) {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	m := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := m[s]; ok {
			continue
		}
		m[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func baseNames(paths ...string) []string {
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		b := filepath.Base(filepath.ToSlash(p))
		if b != "" {
			out = append(out, b)
		}
	}
	return out
}
