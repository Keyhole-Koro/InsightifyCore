package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"insightify/internal/llm"
	"insightify/internal/pipeline"
	"insightify/internal/scan"
	t "insightify/internal/types"
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
				// Scan for m0: depth-limited to 2
				opts := scan.Options{MaxDepth: 2}
				ext := map[string]int{}
				var idx []t.FileIndexEntry
				_ = scan.ScanWithOptions(env.Repo, opts, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					ext[f.Ext]++
					idx = append(idx, t.FileIndexEntry{Path: f.Path, Size: f.Size})
				})
				// Build d1/d2 directory sets
				d1set := map[string]struct{}{}
				d2set := map[string]struct{}{}
				for _, it := range idx {
					dir := filepath.ToSlash(filepath.Dir(it.Path))
					if dir == "." || dir == "" {
						continue
					}
					segs := strings.Split(dir, "/")
					if len(segs) >= 1 {
						d1set[segs[0]] = struct{}{}
					}
					if len(segs) >= 2 {
						d2set[segs[0]+"/"+segs[1]] = struct{}{}
					}
				}
				var d1, d2 []string
				for k := range d1set {
					d1 = append(d1, k)
				}
				for k := range d2set {
					d2 = append(d2, k)
				}
				return t.M0In{ExtCounts: ext, DirsDepth1: d1, DirsDepth2: d2}, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "m0")
				p := pipeline.M0{LLM: env.LLM}
				return p.Run(ctx, in.(t.M0In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.M0In
					Salt string
				}{in.(t.M0In), env.ModelSalt})
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
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
					ig = UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
				}
				opts := scan.Options{IgnoreDirs: ig}
				var idx []t.FileIndexEntry
				var mds []t.MDDoc
				_ = scan.ScanWithOptions(env.Repo, opts, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					idx = append(idx, t.FileIndexEntry{Path: f.Path, Size: f.Size})
					if f.Ext == ".md" {
						if b, e := os.ReadFile(f.AbsPath); e == nil {
							txt := string(b)
							txt = env.StripImgMD.ReplaceAllString(txt, "")
							txt = env.StripImgHTML.ReplaceAllString(txt, "")
							mds = append(mds, t.MDDoc{Path: f.Path, Text: txt})
						}
					}
				})
				return t.M1In{
					FileIndex: idx,
					MDDocs:    mds,
					Hints:     &t.M1Hints{},
					Limits:    &t.M1Limits{MaxNext: env.MaxNext},
				}, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "m1")
				p := pipeline.M1{LLM: env.LLM}
				return p.Run(ctx, in.(t.M1In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.M1In
					Salt string
				}{in.(t.M1In), env.ModelSalt})
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
				var m1 t.M1Out
				ReadJSON(env.OutDir, "m1.json", &m1)

				// Prepare opened files and focus questions
				var opened []t.OpenedFile
				var focus []t.FocusQuestion
				picked := 0
				for _, nf := range m1.NextFiles {
					if picked >= env.MaxNext {
						break
					}
					full := filepath.Join(env.Repo, filepath.Clean(nf.Path))
					b, err := os.ReadFile(full)
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

				// Re-scan for context: full depth, ignore library_roots
				var index []t.FileIndexEntry
				var mdDocs []t.MDDoc
				ig2 := []string{}
				var m0prev2 struct {
					LibraryRoots []string `json:"library_roots"`
				}
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev2)
					ig2 = UniqueStrings(baseNames(m0prev2.LibraryRoots...)...)
				}
				opts := scan.Options{IgnoreDirs: ig2}
				_ = scan.ScanWithOptions(env.Repo, opts, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					index = append(index, t.FileIndexEntry{Path: f.Path, Size: f.Size})
					if f.Ext == ".md" {
						if b, e := os.ReadFile(f.AbsPath); e == nil {
							txt := string(b)
							txt = env.StripImgMD.ReplaceAllString(txt, "")
							txt = env.StripImgHTML.ReplaceAllString(txt, "")
							mdDocs = append(mdDocs, t.MDDoc{Path: f.Path, Text: txt})
						}
					}
				})
				var m0c struct {
					MainSourceRoots []string `json:"main_source_roots"`
				}
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0c)
					if len(m0c.MainSourceRoots) > 0 {
						index = FilterIndexByRoots(index, m0c.MainSourceRoots)
						mdDocs = FilterMDDocsByRoots(mdDocs, m0c.MainSourceRoots)
					}
				}

				return t.M2In{
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
				p := pipeline.M2{LLM: env.LLM}
				return p.Run(ctx, in.(t.M2In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.M2In
					Salt string
				}{in.(t.M2In), env.ModelSalt})
			},
			Downstream: nil,
			Strategy:   jsonStrategy{},
		},
	}
}
