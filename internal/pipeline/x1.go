package pipeline

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"insightify/internal/scan"
	t "insightify/internal/types"
)

// X1 builds a dependency graph from extractor specs (X0 output) by scanning files.
// This pipeline does not use an LLM — it is entirely programmatic.
type X1 struct{}

func (X1) Run(ctx context.Context, in t.X1In) (t.X1Out, error) {

	folderNames, err := fetchFolderNames(ctx, in.Repo, in.Roots)
	if err != nil {
		return t.X1Out{}, err
	}

}

func fetchFolderNames(ctx context.Context, repo string, roots t.M0Out) ([]string, error) {
	dirSet := map[string]struct{}{}
	nameSet := map[string]struct{}{}
	var mu sync.Mutex

	sopts := scan.Options{IgnoreDirs: roots.BuildRoots, BypassCache: true}
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
