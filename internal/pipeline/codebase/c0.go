package codebase

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/artifact"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/scan"
)

// C0 prompt — imports/includes only, plus normalization hints for later post-processing.
// This phase does NOT receive source snippets; it must produce RE2 patterns to detect module strings,
// and also provide language-agnostic guidance ("normalize_hints") describing how X1 should derive
// fields like folder_path[], file_name, file_ext, scope/package/subpath, alias, and kind.
// Specs must return both the original extension (with leading dot) and a lowercase canonical_ext value.
const promptC0 = `
You are **Stage C0** of a static-analysis pipeline.

## Role
Given a report of file-extension counts ("ext_counts") and root directories for source and optional runtime configs ("roots"), emit **one spec per language family** present in "ext_counts" (e.g., "js", "py", "go").

## What to produce
- A single JSON object (no comments) with exactly the fields below.
- "familyKeys": maps each family key to a list of **spec keys** that belong to that family (e.g., { "js": ["js","ts","jsx","tsx"] }).
- "specs": one entry per language family found in "ext_counts".

## Grouping rules
- Group related extensions into a **single** spec when they are the same language family (e.g., JavaScript/TypeScript → ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx").
- Only include extensions that are actually present in "ext_counts".
- Every extension listed in "spec.ext" is an **interchangeable** candidate for resolution (priority is handled later; do not add priority fields here).

## Per-spec fields
For each family key "<familyKey>":
- "ext": array of dot-prefixed extensions **as observed** in "ext_counts" (e.g., [".js", ".ts"]).
- "language": array of objects that link a language name and its extensions:
  - Each item: { "name": "<language name>", "ext": ["<dot-prefixed extensions for that language>"] }.
  - Use only extensions that appear in this family/spec.
- "rules.keywords": up to ~8 short tokens that detect import/include-like references.
- "rules.path_split": up to ~6 simple tokens that help split around module paths.
- "normalize_hints.alias": zero or more {"original","normalized"} pairs for later resolution (e.g., from "tsconfig.json" paths or "package.json#imports"). If none, use [].

## Inputs you may rely on
- "ext_counts": map of extension → count.
- "roots": directories that may contain runtime configs (e.g., "tsconfig.json", "package.json").
  - If such configs imply aliases or import maps, surface them in "normalize_hints.alias". Otherwise, use [].

## Output shape (return **exactly** this shape; do not add fields)
{
  "familyKeys": {
    "<familyKey>": ["js","ts","jsx","tsx"]
  },
  "specs": {
    "<familyKey>": {
      "ext": ["<dot-prefixed extensions>"],
      "language": [
        { "name": "<language name>", "ext": "<dot-prefixed extensions>" },
      ],
      "rules": {
        "keywords": ["..."],
        "path_split": ["..."],
        "comment_line_pattern": ["..."], // e.g. ["^\\s*//.*$", "^\\s*#.*$"]
        "comment_block_pattern": ["..."] // e.g. ["/\\*[\\s\\S]*?\\*/", "<!--[\\s\\S]*?-->"]
      },
      "normalize_hints": {
        "alias": [
          { "original": "<alias>", "normalized": "<normalized_name>" }
        ]
      }
    }
  }
}

## Constraints
- Emit specs **only** for families that appear in "ext_counts".
- Use the real family key (e.g., "js", "py", "go") — no placeholders.
- Keep lists concise ("keywords" ≤ ~8, "path_split" ≤ ~6).
- Echo extensions exactly as seen (with leading dot).
- Produce **valid JSON**. No comments. No regex patterns. No fields other than those listed.
- If unknown, use empty arrays instead of inventing values.

`

type C0 struct{ LLM llmclient.LLMClient }

func (x *C0) Run(ctx context.Context, in artifact.C0In) (artifact.C0Out, error) {
	// Populate ext counts if missing so runner BuildInput can stay lightweight.
	if len(in.ExtCounts) == 0 {
		exts, err := computeExtCounts(ctx, in.Repo, in.Roots)
		if err != nil {
			return artifact.C0Out{}, err
		}
		in.ExtCounts = exts
	}

	// add the content of runtime config files to the input later
	raw, err := x.LLM.GenerateJSON(ctx, promptC0, in)
	if err != nil {
		return artifact.C0Out{}, err
	}

	var out artifact.C0Out
	if err := json.Unmarshal(raw, &out); err != nil {
		return artifact.C0Out{}, fmt.Errorf("C0 JSON invalid: %w\nraw: %s", err, string(raw))
	}
	out.Families = out.Families[:0]
	for family, specKeys := range out.FamilyKeys {
		keys := append([]string(nil), specKeys...)
		sort.Strings(keys)
		for _, key := range keys {
			spec, ok := out.Specs[key]
			if !ok {
				continue
			}
			out.Families = append(out.Families, artifact.FamilySpec{
				Family: family,
				Key:    key,
				Spec:   spec,
			})
		}
	}
	sort.Slice(out.Families, func(i, j int) bool {
		if out.Families[i].Family == out.Families[j].Family {
			return out.Families[i].Key < out.Families[j].Key
		}
		return out.Families[i].Family < out.Families[j].Family
	})
	return out, nil
}

func computeExtCounts(ctx context.Context, repo string, roots artifact.M0Out) ([]artifact.ExtCount, error) {
	_ = ctx
	ignore := make(map[string]struct{})
	for _, r := range roots.LibraryRoots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		base := filepath.Base(filepath.ToSlash(r))
		if base == "" {
			continue
		}
		ignore[base] = struct{}{}
	}
	var ignoreDirs []string
	for k := range ignore {
		ignoreDirs = append(ignoreDirs, k)
	}

	extCountMap := map[string]int{}
	if err := scan.ScanWithOptions(repo, scan.Options{IgnoreDirs: ignoreDirs}, func(f scan.FileVisit) {
		if f.IsDir {
			return
		}
		ext := f.Ext
		if ext == "" {
			ext = filepath.Ext(f.Path)
		}
		if ext != "" {
			extCountMap[strings.ToLower(ext)]++
		}
	}); err != nil {
		return nil, err
	}

	extCounts := make([]artifact.ExtCount, 0, len(extCountMap))
	for ext, cnt := range extCountMap {
		extCounts = append(extCounts, artifact.ExtCount{Ext: ext, Count: cnt})
	}
	sort.Slice(extCounts, func(i, j int) bool {
		if extCounts[i].Count == extCounts[j].Count {
			return extCounts[i].Ext < extCounts[j].Ext
		}
		return extCounts[i].Count > extCounts[j].Count
	})
	return extCounts, nil
}