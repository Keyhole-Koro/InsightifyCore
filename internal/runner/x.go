package runner

import (
	"context"
	"path/filepath"
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
				// Scan for x0: full depth, ignore library_roots from m0 when present
				ig := []string{}
				var m0prev struct {
					LibraryRoots []string `json:"library_roots"`
				}
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
					ig = UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
				}
				var idx []t.FileIndexEntry
				counts := map[string]int{}
				_ = scan.ScanWithOptions(env.Repo, scan.Options{IgnoreDirs: ig}, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					idx = append(idx, t.FileIndexEntry{Path: f.Path, Size: f.Size})
					if f.Ext != "" {
						counts[f.Ext]++
					}
				})
				// Build ext counts and sample a few paths
				in := t.X0In{ExtReport: []t.ExtReportEntry{}}
				// Include runtime_config_files from m0 when available
				var m0c struct {
					RuntimeConfigFiles []string `json:"runtime_config_files"`
				}
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0c)
					in.RuntimeConfigFiles = m0c.RuntimeConfigFiles
				}
				for ext, count := range counts {
					if ext == "" {
						continue
					}
					in.ExtReport = append(in.ExtReport, t.ExtReportEntry{
						Ext:   ext,
						Count: count,
					})
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

		// ----- x1 (json-cached) -----
        "x1": {
            Key:  "x1",
            File: "x1.json",
            BuildInput: func(ctx context.Context, env *Env) (any, error) {
				// Load latest x0 specs
				var x0 t.X0Out
				ReadJSON(env.OutDir, "x0.json", &x0)
				// Build fresh index for x1: ignore library_roots from m0 when present
				var index []t.FileIndexEntry
				ig := []string{}
				var m0prev struct {
					LibraryRoots []string `json:"library_roots"`
				}
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0prev)
					ig = UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
				}
				_ = scan.ScanWithOptions(env.Repo, scan.Options{IgnoreDirs: ig}, func(f scan.FileVisit) {
					if f.IsDir {
						return
					}
					index = append(index, t.FileIndexEntry{Path: f.Path, Size: f.Size})
				})
				var m0c struct {
					MainSourceRoots []string `json:"main_source_roots"`
				}
				if FileExists(filepath.Join(env.OutDir, "m0.json")) {
					ReadJSON(env.OutDir, "m0.json", &m0c)
					if len(m0c.MainSourceRoots) > 0 {
						index = FilterIndexByRoots(index, m0c.MainSourceRoots)
					}
				}
				return t.X1In{Repo: env.Repo, Index: index, Specs: x0.Specs}, nil
			},
			Run: func(ctx context.Context, in any, env *Env) (any, error) {
				x := pipelineX1{}
				return x.Run(in.(t.X1In))
			},
			Fingerprint: func(in any, env *Env) string {
				return JSONFingerprint(struct {
					In   t.X1In
					Salt string
				}{in.(t.X1In), env.ModelSalt})
			},
            Downstream: nil,
            Strategy:   jsonStrategy{},
        },

        // ----- x2 (json-cached) -----
        "x2": {
            Key:  "x2",
            File: "x2.json",
            BuildInput: func(ctx context.Context, env *Env) (any, error) {
                // Load x1 output graph
                var x1 t.X1Out
                ReadJSON(env.OutDir, "x1.json", &x1)
                // Build index similarly to x1
                var index []t.FileIndexEntry
                ig := []string{}
                var m0prev struct{ LibraryRoots []string `json:"library_roots"` }
                if FileExists(filepath.Join(env.OutDir, "m0.json")) {
                    ReadJSON(env.OutDir, "m0.json", &m0prev)
                    ig = UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
                }
                _ = scan.ScanWithOptions(env.Repo, scan.Options{IgnoreDirs: ig}, func(f scan.FileVisit) {
                    if f.IsDir { return }
                    index = append(index, t.FileIndexEntry{Path: f.Path, Size: f.Size})
                })
                var m0c struct{ MainSourceRoots []string `json:"main_source_roots"` }
                if FileExists(filepath.Join(env.OutDir, "m0.json")) {
                    ReadJSON(env.OutDir, "m0.json", &m0c)
                    if len(m0c.MainSourceRoots) > 0 {
                        index = FilterIndexByRoots(index, m0c.MainSourceRoots)
                    }
                }
                return t.X2In{Index: index, Graph: x1}, nil
            },
            Run: func(ctx context.Context, in any, env *Env) (any, error) {
                x := pipelineX2{}
                return x.Run(in.(t.X2In))
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
type pipelineX2 struct{}

func (p pipelineX0) Run(ctx context.Context, in t.X0In) (t.X0Out, error) {
	real := pipeline.X0{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineX1) Run(in t.X1In) (t.X1Out, error) {
    real := pipeline.X1{}
    return real.Run(in)
}
func (pipelineX2) Run(in t.X2In) (t.X2Out, error) {
    real := pipeline.X2{}
    return real.Run(in)
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
