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

// BuildX defines x0/x1 using the same registry/strategy system.
// x0 uses versionedStrategy; x1 uses jsonStrategy.
func BuildX(env *Env) map[string]PhaseSpec {
	return map[string]PhaseSpec{
		// ----- x0 (versioned) -----
		"x0": {
			Key:  "x0",
			File: "x0.json", // latest pointer; versioned writes also occur
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

				// Build X0 input. Align this with your actual t.X0In type.
				in := t.X0In{
					Repo:      env.Repo,
					ExtCounts: extCounts, // if your X0In expects a map, adjust accordingly (but you lose determinism).
					Roots:     m0prev,
				}
				return in, nil
			},

			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "x0")
				x := pipelineX0{LLM: env.LLM}
				return x.Run(ctx, in.(t.X0In))
			},
			Fingerprint: func(in any, env *Env) string {
				// Even though versioned, keep a meta fingerprint for traceability.
				return JSONFingerprint(struct {
					In   t.X0In
					Salt string
				}{in.(t.X0In), env.ModelSalt})
			},
			Downstream: []string{"x1"},
			Strategy:   versionedStrategy{},
		},
		// ----- x1 (json) -----
		"x1": {
			Key:  "x1",
			File: "x1.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				var x0prev t.X0Out
				if FileExists(filepath.Join(env.OutDir, "x0.json")) {
					ReadJSON(env.OutDir, "x0.json", &x0prev)
				}
				var m0prev t.M0Out
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
				}
				in := t.X1In{
					Repo:  env.Repo,
					Specs: x0prev.Specs,
					Roots: m0prev,
				}
				return in, nil
			},

			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "x1")
				x := pipelineX1{}
				return x.Run(ctx, in.(t.X1In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.X1In
					Salt string
				}{in.(t.X1In), env.ModelSalt})
			},
			Downstream: []string{"x2"},
			Strategy:   jsonStrategy{},
		},
		// ----- x2 (json) -----
		"x2": {
			Key:  "x2",
			File: "x2.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				// Wire x1 output (import statement ranges) as input to x2
				var x1prev t.X1Out
				if FileExists(filepath.Join(env.OutDir, "x1.json")) {
					ReadJSON(env.OutDir, "x1.json", &x1prev)
				}
				log.Printf("x2: read x1.json content: %+v", x1prev)
				log.Printf("x2: read %d import statement groups from x1", len(x1prev.ImportStatementRanges))
				in := t.X2In{Repo: env.Repo, Stmts: x1prev.ImportStatementRanges}
				return in, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "x2")
				x := pipelineX2{LLM: env.LLM}
				return x.Run(ctx, in.(t.X2In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.X2In
					Salt string
				}{in.(t.X2In), env.ModelSalt})
			},
			Downstream: nil,
			Strategy:   jsonStrategy{},
		},
	}
}

// thin adapters
type pipelineX0 struct{ LLM llm.LLMClient }
type pipelineX1 struct{}
type pipelineX2 struct{ LLM llm.LLMClient }

func (p pipelineX0) Run(ctx context.Context, in t.X0In) (t.X0Out, error) {
	real := pipeline.X0{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineX1) Run(ctx context.Context, in t.X1In) (t.X1Out, error) {
	real := pipeline.X1{}
	return real.Run(ctx, in)
}
func (p pipelineX2) Run(ctx context.Context, in t.X2In) (t.X2Out, error) {
	real := pipeline.X2{LLM: p.LLM}
	return real.Run(ctx, in)
}

// helpers for x0
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
