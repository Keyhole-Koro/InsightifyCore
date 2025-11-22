package mainline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	llmclient "insightify/internal/llmClient"
	"insightify/internal/scan"
	"insightify/internal/snippet"
	t "insightify/internal/types"
	ml "insightify/internal/types/mainline"
	"insightify/internal/wordidx"
)

// Preamble is assumed to be defined elsewhere as `prologue`. We rely on it.
// M2 now focuses on delta-only updates; updated_hypothesis is not re-emitted.
const promptM2 = prologue + `

You MUST output STRICT JSON that exactly matches the schema below.
No comments, no trailing commas, no ellipses “…”, no backticks.
If something is unknown, return null or an empty array/string explicitly.
Paths must be repository-relative. Do not invent files, symbols, or line ranges.

Schema:
{
  "delta": {                                      // Changes vs previous hypothesis (from m1 or prior m2)
    "added":   ["string"],
    "removed": ["string"],
    "modified": [
      {
        "field": "string",                        // e.g., "architecture.summary" or "architecture.key_components[2].responsibility"
        "before": any,                            // Use strings if unsure
        "after":  any
      }
    ]
  },

  "needs_input": ["string"],                       // Questions for the human
  "stop_when":  ["string"],                        // Convergence criteria
  "notes":      ["string"]                         // Misc short notes
}

Rules & Guidance:
- No fixed vocabularies: use precise, observed wording.
- Delta reflects how you would change the current architecture draft. Only include real differences.
- If any component has low confidence or unknowns, emit actionable items in "needs_input".
  - Use clear directives such as:
    - "snippet:path=<file> identifier=<name> reason=<...>"
    - "wordsearch:term=<token> hint_path=<folder or file> reason=<...>"
- JSON only. Do not include Markdown, code fences, or prose outside the JSON object.

`

type M2 struct {
	LLM             llmclient.LLMClient
	SnippetProvider snippet.Provider
	WordIndex       *wordidx.AggIndex
}

// Run executes M2 with robust JSON handling, including up to 5 iterations to resolve needs_input.
func (p *M2) Run(ctx context.Context, in ml.M2In) (ml.M2Out, error) {
	var final ml.M2Out
	var agg ml.Delta
	seenOpened := make(map[string]bool)
	for _, of := range in.OpenedFiles {
		seenOpened[of.Path] = true
	}

	for i := 0; i < 5; i++ {
		out, err := p.runOnce(ctx, in)
		if err != nil {
			return ml.M2Out{}, err
		}
		final = out
		agg = mergeDelta(agg, out.Delta)
		if len(out.NeedsInput) == 0 {
			break
		}
		newFiles := p.fetchInputs(ctx, out.NeedsInput)
		added := 0
		for _, nf := range newFiles {
			if nf.Path == "" || seenOpened[nf.Path] {
				continue
			}
			in.OpenedFiles = append(in.OpenedFiles, nf)
			seenOpened[nf.Path] = true
			added++
		}
		if added == 0 {
			break
		}
	}
	final.Delta = agg
	return final, nil
}

// runOnce executes a single LLM call with current inputs.
func (p *M2) runOnce(ctx context.Context, in ml.M2In) (ml.M2Out, error) {
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

// fetchInputs processes needs_input directives and returns opened files to feed next iteration.
func (p *M2) fetchInputs(ctx context.Context, needs []string) []t.OpenedFile {
	var opened []t.OpenedFile
	for _, n := range needs {
		n = strings.TrimSpace(n)
		switch {
		case strings.HasPrefix(n, "snippet:"):
			req := parseSnippetRequest(n)
			if req.Path == "" || req.Name == "" || p.SnippetProvider == nil {
				continue
			}
			snips, err := p.SnippetProvider.Collect(ctx, snippet.Query{
				Seeds:     []snippet.Identifier{{Path: req.Path, Name: req.Name}},
				MaxTokens: 4000,
			})
			if err != nil {
				continue
			}
			for _, s := range snips {
				opened = append(opened, t.OpenedFile{
					Path:    s.Identifier.Path + "#" + s.Identifier.Name,
					Content: s.Code,
				})
			}
		case strings.HasPrefix(n, "wordsearch:"):
			term := parseWordRequest(n)
			if term == "" || p.WordIndex == nil {
				continue
			}
			refs := p.WordIndex.Find(ctx, term)
			for _, r := range refs {
				if content, err := readFileContent(r.FilePath); err == nil {
					opened = append(opened, t.OpenedFile{Path: r.FilePath, Content: content})
				}
			}
		}
	}
	return opened
}

type snippetReq struct {
	Path string
	Name string
}

func parseSnippetRequest(s string) snippetReq {
	s = strings.TrimPrefix(s, "snippet:")
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
	var req snippetReq
	for _, f := range fields {
		if strings.HasPrefix(f, "path=") {
			req.Path = strings.TrimPrefix(f, "path=")
		} else if strings.HasPrefix(f, "identifier=") {
			req.Name = strings.TrimPrefix(f, "identifier=")
		} else if strings.HasPrefix(f, "name=") {
			req.Name = strings.TrimPrefix(f, "name=")
		}
	}
	return req
}

func parseWordRequest(s string) string {
	s = strings.TrimPrefix(s, "wordsearch:")
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
	for _, p := range parts {
		if strings.HasPrefix(p, "term=") {
			return strings.TrimPrefix(p, "term=")
		}
	}
	return ""
}

func readFileContent(path string) (string, error) {
	f, err := scan.CurrentSafeFS().SafeOpen(path)
	if err != nil {
		// fallback to default FS or direct OS open
		if data, e := os.ReadFile(path); e == nil {
			return string(data), nil
		}
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func mergeDelta(base, add ml.Delta) ml.Delta {
	base.Added = append(base.Added, add.Added...)
	base.Removed = append(base.Removed, add.Removed...)
	base.Modified = append(base.Modified, add.Modified...)
	return base
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
