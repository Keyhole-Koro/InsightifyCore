package codebase

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/safeio"
	"insightify/internal/scan"
	cb "insightify/internal/types/codebase"
	"insightify/internal/wordidx"
)

// ---- Internal models (optional middle-layer types) ----

type TargetCount struct {
	Target string
	Count  int
}

type FileDeps struct {
	Path         string
	TargetCounts []TargetCount
}

// ---- Pipeline ----

type C1 struct{}

// Run executes the C1 pipeline for dependency extraction.
func (C1) Run(ctx context.Context, in cb.C1In) (cb.C1Out, error) {
	log.Printf("C1: starting scan in repo %s", in.Repo)

	var out []cb.Dependencies
	for family, specs := range in.Specs {
		dep, err := Dependencies(ctx, in.Repo, in.Roots.MainSourceRoots, specs.Exts)
		if err != nil {
			return cb.C1Out{}, err
		}
		log.Printf("C1: found %d dependencies for family %s", len(dep.Dependencies), family)
		out = append(out, dep)
	}
	return cb.C1Out{PossibleDependencies: out}, nil
}

// Dependencies scans once for a given (repo, roots, exts) and returns a single cb.Dependencies.
func Dependencies(ctx context.Context, repo string, roots []string, exts []string) (cb.Dependencies, error) {
	fs := scan.CurrentSafeFS()
	if fs == nil {
		fs = safeio.Default()
	}
	if fs == nil {
		return cb.Dependencies{}, fmt.Errorf("c1: safe filesystem not configured")
	}
	base := fs.Root()

	// Resolve search roots
	var resolvedRoots []string
	if len(roots) == 0 {
		resolvedRoots = []string{base}
	} else {
		for _, r := range roots {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			resolvedRoots = append(resolvedRoots, filepath.Join(base, filepath.Clean(r)))
		}
	}
	if len(resolvedRoots) == 0 {
		resolvedRoots = []string{base}
	}

	// Initialize the indexer
	agg := wordidx.New().
		Root(resolvedRoots...).
		Allow(exts...).
		Workers(2).
		Options(scan.Options{BypassCache: true}).
		Start(ctx)

	if err := agg.Wait(ctx); err != nil {
		return cb.Dependencies{}, err
	}

	// Build the filename index for O(1) lookups
	filenameIndex := buildFilenameIndex(ctx, agg)

	// Infer dependencies
	var srcDeps []cb.SourceDependency
	for _, fi := range agg.Files(ctx) {
		from := repoRelative(base, fi.Path)
		counts := make(map[string]int)

		for _, w := range fi.Index.Words {
			tok := strings.ToLower(w.Text)
			if paths, ok := filenameIndex[tok]; ok {
				for p := range paths {
					target := repoRelative(base, p)
					if target == from {
						continue
					}
					counts[target]++
				}
			}
		}

		reqs := keysSorted(counts)
		srcDeps = append(srcDeps, cb.SourceDependency{
			Path:     from,
			Language: "", // fill if you want (derive from ext)
			Ext:      strings.TrimPrefix(filepath.Ext(from), "."),
			Requires: reqs,
		})
	}

	// Sort for deterministic output
	sort.Slice(srcDeps, func(i, j int) bool { return srcDeps[i].Path < srcDeps[j].Path })

	log.Printf("C1: scanned %d files in repo %s", len(srcDeps), repo)
	return cb.Dependencies{
		Repo:         repo,
		Roots:        roots,
		Exts:         exts,
		Dependencies: srcDeps,
	}, nil
}

// ---- Helpers ----

// buildFilenameIndex constructs a fast lookup index mapping tokens to file paths.
// Example for "foo.bar.ts":
//   - "foo.bar.ts"  (basename)
//   - "foo.bar"     (stem = basename without extension)
//   - "foo", "bar", "ts"  (dot-split tokens)
func buildFilenameIndex(ctx context.Context, agg *wordidx.AggIndex) map[string]map[string]struct{} {
	idx := make(map[string]map[string]struct{})

	add := func(token, fullpath string) {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			return
		}
		m := idx[token]
		if m == nil {
			m = make(map[string]struct{})
			idx[token] = m
		}
		m[fullpath] = struct{}{}
	}

	for _, fi := range agg.Files(ctx) {
		base := filepath.Base(fi.Path) // e.g. "foo.bar.ts"

		// (1) Basename
		add(base, fi.Path)

		// (2) Stem (basename without the final extension)
		if ext := filepath.Ext(base); ext != "" {
			stem := base[:len(base)-len(ext)] // e.g. "foo.bar"
			add(stem, fi.Path)
		}

		// (3) Dot-split tokens (e.g. ["foo", "bar", "ts"])
		for _, part := range strings.Split(base, ".") {
			add(part, fi.Path)
		}
	}

	return idx
}

// keysSorted returns map keys in ascending order.
func keysSorted(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func repoRelative(root, path string) string {
	if path == "" {
		return ""
	}
	if root == "" {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
