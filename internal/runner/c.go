package runner

import (
	"context"
	"log"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/llm"
	"insightify/internal/pipeline"
	"insightify/internal/scan"
	t "insightify/internal/types"
)

// BuildX defines c0/c1 using the same registry/strategy system.
// c0 uses versionedStrategy; c1 uses jsonStrategy.
func BuildX(env *Env) map[string]PhaseSpec {
	return map[string]PhaseSpec{
		// ----- c0 (versioned) -----
		"c0": {
			Key:  "c0",
			File: "c0.json", // latest pointer; versioned writes also occur
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				var m0prev t.M0Out
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
				}

				ignore := UniqueStrings(m0prev.LibraryRoots...)

				extCountMap := map[string]int{}
				var idx []t.FileIndexEntry

				if err := scan.ScanWithOptions(env.Repo, scan.Options{IgnoreDirs: ignore}, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					idx = append(idx, t.FileIndexEntry{Path: f.Path, Size: f.Size})

					// Use filepath. as the source of truth; fall back if scan provides none.
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

				extCounts := make([]t.ExtCount, 0, len(extCountMap))
				for ext, cnt := range extCountMap {
					extCounts = append(extCounts, t.ExtCount{Ext: ext, Count: cnt})
				}
				sort.Slice(extCounts, func(i, j int) bool {
					if extCounts[i].Count == extCounts[j].Count {
						return extCounts[i].Ext < extCounts[j].Ext
					}
					return extCounts[i].Count > extCounts[j].Count
				})

				// Build c0 input. Align this with your actual t.C0In type.
				in := t.C0In{
					Repo:      env.Repo,
					ExtCounts: extCounts, // if your c0In expects a map, adjust accordingly (but you lose determinism).
					Roots:     m0prev,
				}
				return in, nil
			},

			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "c0")
				x := pipelineC0{LLM: env.LLM}
				return x.Run(ctx, in.(t.C0In))
			},
			Fingerprint: func(in any, env *Env) string {
				// Even though versioned, keep a meta fingerprint for traceability.
				return JSONFingerprint(struct {
					In   t.C0In
					Salt string
				}{in.(t.C0In), env.ModelSalt})
			},
			Downstream: []string{"c1"},
			Strategy:   versionedStrategy{},
		},
		// ----- c1 (json) -----
		"c1": {
			Key:  "c1",
			File: "c1.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				var c0prev t.C0Out
				if FileExists(filepath.Join(env.OutDir, "c0.json")) {
					ReadJSON(env.OutDir, "c0.json", &c0prev)
				}
				var m0prev t.M0Out
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
				}
				in := t.C1In{
					Repo:  env.Repo,
					Specs: c0prev.Specs,
					Roots: m0prev,
				}
				return in, nil
			},

			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "c1")
				x := pipelineC1{}
				return x.Run(ctx, in.(t.C1In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.C1In
					Salt string
				}{in.(t.C1In), env.ModelSalt})
			},
			Downstream: []string{"c2"},
			Strategy:   jsonStrategy{},
		},
		// ----- c2 (json) -----
		"c2": {
			Key:  "c2",
			File: "c2.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				// Wire c1 output (import statement ranges) as input to c2
				var c1prev t.C1Out
				if FileExists(filepath.Join(env.OutDir, "c1.json")) {
					ReadJSON(env.OutDir, "c1.json", &c1prev)
				}
				log.Printf("c2: read c1.json content: %+v", c1prev)
				log.Printf("c2: read %d import statement groups from c1", len(c1prev.ImportStatementRanges))
				in := t.C2In{Repo: env.Repo, Stmts: c1prev.ImportStatementRanges}
				return in, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "c2")
				x := pipelineC2{LLM: env.LLM}
				return x.Run(ctx, in.(t.C2In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.C2In
					Salt string
				}{in.(t.C2In), env.ModelSalt})
			},
			Downstream: nil,
			Strategy:   jsonStrategy{},
		},
	}
}

// thin adapters
type pipelineC0 struct{ LLM llm.LLMClient }
type pipelineC1 struct{}
type pipelineC2 struct{ LLM llm.LLMClient }

func (p pipelineC0) Run(ctx context.Context, in t.C0In) (t.C0Out, error) {
	real := pipeline.C0{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineC1) Run(ctx context.Context, in t.C1In) (t.C1Out, error) {
	real := pipeline.C1{}
	return real.Run(ctx, in)
}
func (p pipelineC2) Run(ctx context.Context, in t.C2In) (t.C2Out, error) {
	real := pipeline.C2{LLM: p.LLM}
	return real.Run(ctx, in)
}

// helpers for c0
func sampleFilesByExt(ext string, index []t.FileIndexEntry, limit int) (string, []string) {
	if limit <= 0 {
		limit = 3
	}
	var paths []string
	for _, it := range index {
		if strings.HasSuffix(strings.ToLower(it.Path), strings.ToLower(ext)) {
			paths = append(paths, it.Path)
			if len(paths) >= limit {
				break
			}
		}
	}
	return "", paths
}
