package runner

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	codepipe "insightify/internal/pipeline/codebase"
	"insightify/internal/scan"

	baset "insightify/internal/types"
	cb "insightify/internal/types/codebase"
	ml "insightify/internal/types/mainline"
)

// BuildC defines c0/c1 using the same registry/strategy system.
// c0 uses versionedStrategy; c1 uses jsonStrategy.
func BuildC(env *Env) map[string]PhaseSpec {
	return map[string]PhaseSpec{
		// ----- c0 (versioned) -----
		"c0": {
			Key:  "c0",
			File: "c0.json", // latest pointer; versioned writes also occur
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				var m0prev ml.M0Out
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
				}

				ignore := UniqueStrings(m0prev.LibraryRoots...)

				extCountMap := map[string]int{}
				var idx []baset.FileIndexEntry

				if err := scan.ScanWithOptions(env.Repo, scan.Options{IgnoreDirs: ignore}, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					idx = append(idx, baset.FileIndexEntry{Path: f.Path, Size: f.Size})

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

				extCounts := make([]baset.ExtCount, 0, len(extCountMap))
				for ext, cnt := range extCountMap {
					extCounts = append(extCounts, baset.ExtCount{Ext: ext, Count: cnt})
				}
				sort.Slice(extCounts, func(i, j int) bool {
					if extCounts[i].Count == extCounts[j].Count {
						return extCounts[i].Ext < extCounts[j].Ext
					}
					return extCounts[i].Count > extCounts[j].Count
				})

				// Build c0 input. Align this with your actual t.C0In type.
				in := cb.C0In{
					Repo:      env.Repo,
					ExtCounts: extCounts, // if your c0In expects a map, adjust accordingly (but you lose determinism).
					Roots:     m0prev,
				}
				return in, nil
			},

			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "c0")
				x := pipelineC0{LLM: env.LLM}
				return x.Run(ctx, in.(cb.C0In))
			},
			Fingerprint: func(in any, env *Env) string {
				// Even though versioned, keep a meta fingerprint for traceability.
				return JSONFingerprint(struct {
					In   cb.C0In
					Salt string
				}{in.(cb.C0In), env.ModelSalt})
			},
			Downstream: []string{"c1"},
			Strategy:   versionedStrategy{},
		},
		// ----- c1 (json) -----
		"c1": {
			Key:  "c1",
			File: "c1.json",
			BuildInput: func(ctx context.Context, env *Env) (any, error) {
				var c0prev cb.C0Out
				if FileExists(filepath.Join(env.OutDir, "c0.json")) {
					ReadJSON(env.OutDir, "c0.json", &c0prev)
				}
				var m0prev ml.M0Out
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
				}
				in := cb.C1In{
					Repo:  env.Repo,
					Specs: c0prev.Specs,
					Roots: m0prev,
				}
				return in, nil
			},

			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				ctx = llm.WithPhase(ctx, "c1")
				x := pipelineC1{}
				return x.Run(ctx, in.(cb.C1In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   cb.C1In
					Salt string
				}{in.(cb.C1In), env.ModelSalt})
			},
			Downstream: []string{"c2"},
			Strategy:   jsonStrategy{},
		},
	}
}

// thin adapters
type pipelineC0 struct{ LLM llmclient.LLMClient }
type pipelineC1 struct{}
type pipelineC2 struct{}

func (p pipelineC0) Run(ctx context.Context, in cb.C0In) (cb.C0Out, error) {
	real := codepipe.C0{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineC1) Run(ctx context.Context, in cb.C1In) (cb.C1Out, error) {
	real := codepipe.C1{}
	return real.Run(ctx, in)
}
