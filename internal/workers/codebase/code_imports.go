package codebase

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/safeio"
	"insightify/internal/scan"

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

type CodeImports struct{}

// Run executes the CodeImports pipeline for dependency extraction.
func (CodeImports) Run(ctx context.Context, in artifact.CodeImportsIn) (artifact.CodeImportsOut, error) {
	log.Printf("CodeImports: starting scan in repo %s", in.Repo)

	var out []artifact.Dependencies
	for _, fam := range in.Families {
		dep, err := ScanDependencies(ctx, in.Repo, in.Roots.MainSourceRoots, fam)
		if err != nil {
			return artifact.CodeImportsOut{}, err
		}
		log.Printf("CodeImports: found %d dependencies for family %s (%s)", len(dep.Files), fam.Family, fam.Key)
		out = append(out, dep)
	}
	return artifact.CodeImportsOut{PossibleDependencies: out}, nil
}

// Dependencies scans once for a given (repo, roots, exts) and returns a single Dependencies.
func ScanDependencies(ctx context.Context, repo string, roots []string, family artifact.FamilySpec) (artifact.Dependencies, error) {
	fs := scan.CurrentSafeFS()
	if fs == nil {
		fs = safeio.Default()
	}
	if fs == nil {
		return artifact.Dependencies{}, fmt.Errorf("codeImports: safe filesystem not configured")
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
		Allow(family.Spec.Exts...).
		Workers(2).
		Options(scan.Options{BypassCache: true}).
		Start(ctx)

	if err := agg.Wait(ctx); err != nil {
		return artifact.Dependencies{}, err
	}

	// Build the filename index for O(1) lookups
	filenameIndex := buildFilenameIndex(ctx, agg)

	// Infer dependencies
	var srcDeps []artifact.SourceDependency
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
		reqRefs := make([]artifact.FileRef, 0, len(reqs))
		for _, req := range reqs {
			reqRefs = append(reqRefs, artifact.NewFileRef(req))
		}
		srcDeps = append(srcDeps, artifact.SourceDependency{
			File:     artifact.NewFileRef(from),
			Language: "",
			Requires: reqRefs,
		})
	}

	// Sort for deterministic output
	sort.Slice(srcDeps, func(i, j int) bool { return srcDeps[i].File.Path < srcDeps[j].File.Path })

	log.Printf("CodeImports: scanned %d files in repo %s", len(srcDeps), repo)
	return artifact.Dependencies{
		Repo:    repo,
		Roots:   roots,
		Exts:    family.Spec.Exts,
		Family:  family.Family,
		SpecKey: family.Key,
		Files:   srcDeps,
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