package runner

import (
	"context"
	"path/filepath"

	"insightify/internal/llm"
	mlpipe "insightify/internal/pipeline/mainline"
	"insightify/internal/scan"
	baset "insightify/internal/types"
	ml "insightify/internal/types/mainline"
)

// BuildRegistry defines m0/m1/m2 in one place.
// Add/modify phases here without touching main or execution logic.
func BuildRegistry(env *Env) map[string]PhaseSpec {
	return map[string]PhaseSpec{
		// ----- m0 -----
		"m0": {
			Key:  "m0",
			File: "m0.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				// Scan for m0: depth-limited to 1
				opts := scan.Options{MaxDepth: 1}
				extCounts := map[string]int{}
				var idx []baset.FileIndexEntry
				_ = scan.ScanWithOptions(env.Repo, opts, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					extCounts[f.Ext]++
					idx = append(idx, baset.FileIndexEntry{Path: f.Path, Size: f.Size})
				})
				// Build set of repo-relative directories (any depth) encountered during the scan
				dirSet := map[string]struct{}{}
				for _, it := range idx {
					dir := filepath.ToSlash(filepath.Dir(it.Path))
					if dir == "." || dir == "" {
						continue
					}
					dirSet[dir] = struct{}{}
				}
				var dirs []string
				for k := range dirSet {
					dirs = append(dirs, k)
				}
				return ml.M0In{ExtCounts: extCounts, DirsDepth1: dirs}, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "m0")
				p := mlpipe.M0{LLM: env.LLM}
				return p.Run(ctx, in.(ml.M0In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   ml.M0In
					Salt string
				}{in.(ml.M0In), env.ModelSalt})
			},
			Downstream: []string{"m1", "m2"},
			Strategy:   jsonStrategy{},
		},

		// ----- m1 -----
		"m1": {
			Key:  "m1",
			File: "m1.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				// Scan for m1: full depth, ignore library_roots from m0 if present
				ig := []string{}
				var m0prev struct {
					LibraryRoots []string `json:"library_roots"`
				}
				if FileExists(env.ArtifactFS, filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.ArtifactFS, env.OutDir, "m0.json", &m0prev)
					ig = UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
				}
				opts := scan.Options{IgnoreDirs: ig}
				var idx []baset.FileIndexEntry
				var mds []baset.MDDoc
				_ = scan.ScanWithOptions(env.Repo, opts, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					idx = append(idx, baset.FileIndexEntry{Path: f.Path, Size: f.Size})
					if f.Ext == ".md" {
						if b, e := ensureFS(env.RepoFS).SafeReadFile(f.AbsPath); e == nil {
							txt := string(b)
							txt = env.StripImgMD.ReplaceAllString(txt, "")
							txt = env.StripImgHTML.ReplaceAllString(txt, "")
							mds = append(mds, baset.MDDoc{Path: f.Path, Text: txt})
						}
					}
				})
				return ml.M1In{
					FileIndex: idx,
					MDDocs:    mds,
					Hints:     &ml.M1Hints{},
					Limits:    &ml.M1Limits{MaxNext: env.MaxNext},
				}, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "m1")
				p := mlpipe.M1{LLM: env.LLM}
				return p.Run(ctx, in.(ml.M1In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   ml.M1In
					Salt string
				}{in.(ml.M1In), env.ModelSalt})
			},
			Downstream: []string{"m2"},
			Strategy:   jsonStrategy{},
		},

		// ----- m2 -----
		"m2": {
			Key:  "m2",
			File: "m2.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				// Requires m1 output
				var m1 ml.M1Out
				ReadJSON(env.ArtifactFS, env.OutDir, "m1.json", &m1)

				// Prepare opened files and focus questions
				var opened []baset.OpenedFile
				var focus []baset.FocusQuestion
				picked := 0
				for _, nf := range m1.NextFiles {
					if picked >= env.MaxNext {
						break
					}
					full := filepath.Join(env.RepoRoot, filepath.Clean(nf.Path))
					b, err := ensureFS(env.RepoFS).SafeReadFile(full)
					if err != nil {
						continue
					}
					opened = append(opened, baset.OpenedFile{Path: nf.Path, Content: string(b)})
					if len(nf.WhatToCheck) == 0 {
						focus = append(focus, baset.FocusQuestion{Path: nf.Path, Question: "Review this file for key architecture details"})
					} else {
						for _, q := range nf.WhatToCheck {
							focus = append(focus, baset.FocusQuestion{Path: nf.Path, Question: q})
						}
					}
					picked++
				}

				// Re-scan for context: full depth, ignore library_roots
				var index []baset.FileIndexEntry
				var mdDocs []baset.MDDoc
				ig2 := []string{}
				var m0prev2 struct {
					LibraryRoots []string `json:"library_roots"`
				}
				if FileExists(env.ArtifactFS, filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.ArtifactFS, env.OutDir, "m0.json", &m0prev2)
					ig2 = UniqueStrings(baseNames(m0prev2.LibraryRoots...)...)
				}
				opts := scan.Options{IgnoreDirs: ig2}
				_ = scan.ScanWithOptions(env.Repo, opts, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					index = append(index, baset.FileIndexEntry{Path: f.Path, Size: f.Size})
					if f.Ext == ".md" {
						if b, e := ensureFS(env.RepoFS).SafeReadFile(f.AbsPath); e == nil {
							txt := string(b)
							txt = env.StripImgMD.ReplaceAllString(txt, "")
							txt = env.StripImgHTML.ReplaceAllString(txt, "")
							mdDocs = append(mdDocs, baset.MDDoc{Path: f.Path, Text: txt})
						}
					}
				})
				var m0c struct {
					MainSourceRoots []string `json:"main_source_roots"`
				}
				if FileExists(env.ArtifactFS, filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.ArtifactFS, env.OutDir, "m0.json", &m0c)
					if len(m0c.MainSourceRoots) > 0 {
						index = FilterIndexByRoots(index, m0c.MainSourceRoots)
						mdDocs = FilterMDDocsByRoots(mdDocs, m0c.MainSourceRoots)
					}
				}

				return ml.M2In{
					Previous:     m1,
					OpenedFiles:  opened,
					Focus:        focus,
					FileIndex:    index,
					MDDocs:       mdDocs[:Min(len(mdDocs), 4)],
					LimitMaxNext: env.MaxNext,
				}, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "m2")
				p := mlpipe.M2{LLM: env.LLM}
				return p.Run(ctx, in.(ml.M2In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   ml.M2In
					Salt string
				}{in.(ml.M2In), env.ModelSalt})
			},
			Downstream: nil,
			Strategy:   jsonStrategy{},
		},
	}
}
