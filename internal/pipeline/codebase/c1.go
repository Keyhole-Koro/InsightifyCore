package codebase

import (
	"context"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/scan"
	cb "insightify/internal/types/codebase"
)

// C1 builds a dependency graph from extractor specs (X0 output) by scanning files.
// This pipeline does not use an LLM — it is entirely programmatic.
type C1 struct{}

func (C1) Run(ctx context.Context, in cb.C1In) (cb.C1Out, error) {
	log.Printf("C1: starting scan in repo %s", in.Repo)

	for family, specs := range in.Specs {
		targetFiles, err := FilesWithExtensions(in.Repo, in.Roots.MainSourceRoots, specs.Ext, scan.Options{})
		if err != nil {
			return cb.C1Out{}, err
		}
		log.Printf("C1: found %d files for family %s", len(targetFiles), family)
	}

	return cb.C1Out{PossibleDependencies: nil}, nil
}

func FilesWithExtensions(repo string, roots []string, exts []string, opts scan.Options) ([]string, error) {
	if len(exts) == 0 || len(roots) == 0 {
		return nil, nil
	}

	repoDir := strings.TrimSpace(repo)
	if repoDir == "" {
		repoDir = "."
	}
	repoAbs, err := filepath.Abs(repoDir)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var files []string

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		rootPath := root
		if !filepath.IsAbs(rootPath) {
			rootPath = filepath.Join(repoAbs, rootPath)
		}
		rootPath = filepath.Clean(rootPath)

		matches, err := scan.FilesWithExtensions(rootPath, exts, opts)
		if err != nil {
			return nil, err
		}

		relPrefix, err := filepath.Rel(repoAbs, rootPath)
		if err != nil {
			relPrefix = rootPath
		}
		relPrefix = filepath.ToSlash(relPrefix)

		for _, match := range matches {
			repoRel := match
			if relPrefix != "." && relPrefix != "" {
				repoRel = filepath.ToSlash(filepath.Join(relPrefix, match))
			}
			repoRel = strings.TrimPrefix(repoRel, "./")
			if repoRel == "" {
				continue
			}
			if _, dup := seen[repoRel]; dup {
				continue
			}
			seen[repoRel] = struct{}{}
			files = append(files, repoRel)
		}
	}

	sort.Strings(files)
	return files, nil
}
