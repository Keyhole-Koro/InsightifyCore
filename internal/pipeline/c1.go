package pipeline

import (
	"context"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"insightify/internal/scan"
	t "insightify/internal/types"
	"insightify/internal/wordidx"
)

// C1 builds a dependency graph from extractor specs (X0 output) by scanning files.
// This pipeline does not use an LLM — it is entirely programmatic.
type C1 struct{}

func (C1) Run(ctx context.Context, in t.C1In) (t.C1Out, error) {
	log.Printf("C1: starting scan in repo %s", in.Repo)
	// 1) Collect folder basenames (used as identifier candidates)
	folderNames, err := fetchFolderNames(ctx, in.Repo, in.Roots)
	if err != nil {
		return t.C1Out{}, err
	}

	log.Printf("C1: found %d unique folder names", len(folderNames))

	// 2) Build IgnoreDirs as basenames (scan compares basenames)
	igAll := make([]string, 0, len(in.Roots.BuildRoots)+len(in.Roots.RuntimeConfigRoots)+len(in.Roots.ConfigRoots)+len(in.Roots.LibraryRoots))
	igAll = append(igAll, in.Roots.BuildRoots...)
	igAll = append(igAll, in.Roots.RuntimeConfigRoots...)
	igAll = append(igAll, in.Roots.ConfigRoots...)
	igAll = append(igAll, in.Roots.LibraryRoots...)
	seenBase := map[string]struct{}{}
	var ignoreBases []string
	for _, p := range igAll {
		b := filepath.Base(filepath.ToSlash(strings.TrimSpace(p)))
		if b == "" {
			continue
		}
		if _, ok := seenBase[b]; ok {
			continue
		}
		seenBase[b] = struct{}{}
		ignoreBases = append(ignoreBases, b)
	}

	// 3) Filter indexed files by extensions present in specs
	var exts []string
	seenExt := map[string]struct{}{}
	for _, s := range in.Specs {
		e := strings.TrimSpace(strings.ToLower(s.Ext))
		if e == "" {
			continue
		}
		if strings.HasPrefix(e, ".") {
			e = e[1:]
		}
		if _, ok := seenExt[e]; ok {
			continue
		}
		seenExt[e] = struct{}{}
		exts = append(exts, e)
	}

	aggIndex := wordidx.NewAgg()
	filter := wordidx.ExtAllow(exts...)
	aggIndex.StartFromScan(ctx, in.Repo, scan.Options{IgnoreDirs: ignoreBases}, 4, filter)

	var found []wordidx.PosRef // e.g. folder names likely to be modules
	for _, f := range folderNames {
		found = append(found, aggIndex.Find(ctx, f)...)
	}

	// 4) Build results per extension/spec using only that spec's keywords
	var groups []t.PerExtImportStatement
	for _, spec := range in.Specs {
		var targets []wordidx.PosRef // e.g. import, require, from, etc.
		for _, k := range spec.Rules.Keywords {
			targets = append(targets, aggIndex.Find(ctx, k)...)
		}
		ranges := importStatementExtractor(targets, found, 8)
		if len(ranges) == 0 {
			continue
		}
		groups = append(groups, t.PerExtImportStatement{Ext: spec.Ext, StmtRange: ranges})
	}
	return t.C1Out{ImportStatementRanges: groups}, nil
}

// not explicitly processed; improve later

// importStatementExtractor keeps the original signature and delegates to the configurable version.
// Defaults:
// - identifier magnitude = max(1, magnitude/2)
// - threshold            = max(1, magnitude/2)
func importStatementExtractor(
	keywordsPosition []wordidx.PosRef,
	identifierPos []wordidx.PosRef,
	magnitude int,
) []t.ImportStatementRange {
	if magnitude <= 0 {
		return nil
	}
	idMag := magnitude / 2
	if idMag < 1 {
		idMag = 1
	}
	thr := magnitude
	if thr < 1 {
		thr = 1
	}
	return importStatementExtractorWithThreshold(keywordsPosition, identifierPos, magnitude, idMag, thr)
}

// importStatementExtractorWithThreshold builds decay arrays with separate magnitudes for
// keywords and identifiers, then returns continuous ranges whose combined weight >= threshold.
func importStatementExtractorWithThreshold(
	keywordsPosition []wordidx.PosRef,
	identifierPos []wordidx.PosRef,
	keywordMagnitude int,
	identifierMagnitude int,
	threshold int,
) []t.ImportStatementRange {
	if keywordMagnitude <= 0 || identifierMagnitude <= 0 || threshold <= 0 {
		return nil
	}

	// 1) Group lines by file and track maximum line
	type perFile struct {
		maxLine      int
		keywordPeaks []int
		identPeaks   []int
	}
	files := make(map[string]*perFile)

	addPeak := func(path string, line int, isKeyword bool) {
		if line <= 0 {
			return
		}
		p := files[path]
		if p == nil {
			p = &perFile{}
			files[path] = p
		}
		if isKeyword {
			p.keywordPeaks = append(p.keywordPeaks, line)
		} else {
			p.identPeaks = append(p.identPeaks, line)
		}
		if line > p.maxLine {
			p.maxLine = line
		}
	}

	for _, k := range keywordsPosition {
		addPeak(k.FilePath, k.Line, true)
	}
	for _, id := range identifierPos {
		addPeak(id.FilePath, id.Line, false)
	}

	// 2) For each file, build a combined weight array (1-based; index 0 unused)
	out := make([]t.ImportStatementRange, 0, len(files))

	for path, info := range files {
		maxMag := keywordMagnitude
		if identifierMagnitude > maxMag {
			maxMag = identifierMagnitude
		}
		n := info.maxLine + maxMag + 1
		if n < 2 {
			n = 2
		}
		weights := make([]int, n) // 1..n-1 used

		// Apply linear decay from each peak, ADDING contributions (keywords and identifiers).
		applyDecayAdd(weights, info.keywordPeaks, keywordMagnitude)
		applyDecayAdd(weights, info.identPeaks, identifierMagnitude)

		// 3) Extract continuous ranges where weight >= threshold
		ranges := rangesFromThreshold(weights, threshold)
		for _, rg := range ranges {
			out = append(out, t.ImportStatementRange{
				FilePath:  path,
				StartLine: rg.s,
				EndLine:   rg.e,
			})
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// applyDecayAdd spreads a peak of `magnitude` at each given line, decreasing by 1 per step
// to both sides until it reaches 0, ADDING contributions onto the `arr`.
// `arr` is 1-based; index 0 is unused.
func applyDecayAdd(arr []int, peaks []int, magnitude int) {
	if magnitude <= 0 {
		return
	}
	n := len(arr)

	for _, center := range peaks {
		if center <= 0 || center >= n {
			continue
		}

		// Center
		arr[center] += magnitude

		// Left decay
		val := magnitude
		for i := center - 1; i >= 1; i-- {
			val--
			if val <= 0 {
				break
			}
			arr[i] += val
		}

		// Right decay
		val = magnitude
		for i := center + 1; i < n; i++ {
			val--
			if val <= 0 {
				break
			}
			arr[i] += val
		}
	}
}

type rng struct{ s, e int }

// rangesFromThreshold scans the 1-based array and returns maximal contiguous ranges
// where arr[i] >= threshold.
func rangesFromThreshold(arr []int, threshold int) []rng {
	n := len(arr)
	out := make([]rng, 0)
	i := 1
	for i < n {
		// Skip values below threshold
		for i < n && arr[i] < threshold {
			i++
		}
		if i >= n {
			break
		}
		// Start of a range
		start := i
		for i < n && arr[i] >= threshold {
			i++
		}
		end := i - 1
		out = append(out, rng{s: start, e: end})
	}
	return out
}

func fetchFolderNames(ctx context.Context, repo string, roots t.M0Out) ([]string, error) {
	dirSet := map[string]struct{}{}
	nameSet := map[string]struct{}{}
	var mu sync.Mutex

	// Build ignore basenames: scan ignores by basename, not full paths.
	all := make([]string, 0, len(roots.BuildRoots)+len(roots.RuntimeConfigRoots)+len(roots.ConfigRoots)+len(roots.LibraryRoots))
	all = append(all, roots.BuildRoots...)
	all = append(all, roots.RuntimeConfigRoots...)
	all = append(all, roots.ConfigRoots...)
	all = append(all, roots.LibraryRoots...)
	seen := map[string]struct{}{}
	var ignoreBases []string
	for _, p := range all {
		b := filepath.Base(filepath.ToSlash(strings.TrimSpace(p)))
		if b == "" {
			continue
		}
		if _, ok := seen[b]; ok {
			continue
		}
		seen[b] = struct{}{}
		ignoreBases = append(ignoreBases, b)
	}

	// Use subtree caching so repeated calls are fast; do not bypass cache.
	sopts := scan.Options{IgnoreDirs: ignoreBases, CacheSubtrees: true}
	_ = scan.ScanWithOptions(repo, sopts, func(f scan.FileVisit) {
		if !f.IsDir {
			return
		}
		if f.Path == "." || f.Path == "" {
			return
		}
		p := filepath.ToSlash(f.Path)
		mu.Lock()
		dirSet[p] = struct{}{}
		base := filepath.Base(p)
		base = strings.TrimSpace(base)
		if base != "" {
			nameSet[base] = struct{}{}
		}
		mu.Unlock()
	})

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)

	return names, nil
}
