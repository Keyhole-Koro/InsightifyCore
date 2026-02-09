package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/llmtool"
	codepipe "insightify/internal/workers/codebase"
)

// BuildRegistryCodebase defines code_roots-code_symbols.
// code_roots uses versionedStrategy; c1 uses jsonStrategy; etc.
func BuildRegistryCodebase(env *Env) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}
	reg["code_roots"] = WorkerSpec{
		Key:         "code_roots",
		Description: "Scan repo layout and ask LLM to classify main source roots, library/vendor roots, and config hotspots.",
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			return artifact.CodeRootsIn{Repo: deps.Repo()}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "code_roots")
			x := pipelineCodeRoots{LLM: env.LLM, Tools: env.MCP}
			out, err := x.Run(ctx, in.(artifact.CodeRootsIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.CodeRootsIn
				Salt string
			}{in.(artifact.CodeRootsIn), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	reg["code_specs"] = WorkerSpec{
		Key:         "code_specs",
		Requires:    []string{"code_roots"},
		Description: "LLM infers language families/import heuristics from extension counts and roots.",
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var codeRootsPrev artifact.CodeRootsOut
			if err := deps.Artifact("code_roots", &codeRootsPrev); err != nil {
				return nil, err
			}
			in := artifact.CodeSpecsIn{
				Repo:  deps.Repo(),
				Roots: codeRootsPrev,
			}
			return in, nil
		},

		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "code_specs")
			x := pipelineCodeSpecs{LLM: env.LLM}
			out, err := x.Run(ctx, in.(artifact.CodeSpecsIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			// Even though versioned, keep a meta fingerprint for traceability.
			return JSONFingerprint(struct {
				In   artifact.CodeSpecsIn
				Salt string
			}{in.(artifact.CodeSpecsIn), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	reg["code_imports"] = WorkerSpec{
		Key:         "code_imports",
		Requires:    []string{"code_specs", "code_roots"},
		Description: "Word-index dependency sweep across source roots to collect possible file-level dependencies.",
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var codeSpecsPrev artifact.CodeSpecsOut
			if err := deps.Artifact("code_specs", &codeSpecsPrev); err != nil {
				return nil, err
			}
			var codeRootsPrev artifact.CodeRootsOut
			if err := deps.Artifact("code_roots", &codeRootsPrev); err != nil {
				return nil, err
			}
			in := artifact.CodeImportsIn{
				Repo:     deps.Repo(),
				Families: codeSpecsPrev.Families,
				Roots:    codeRootsPrev,
			}
			return in, nil
		},

		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "code_imports")
			x := pipelineCodeImports{}
			out, err := x.Run(ctx, in.(artifact.CodeImportsIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.CodeImportsIn
				Salt string
			}{in.(artifact.CodeImportsIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["code_graph"] = WorkerSpec{
		Key:         "code_graph",
		Requires:    []string{"code_imports"},
		Description: "Normalize dependency hits into a DAG and drop weaker bidirectional edges.",
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var codeImportsOut artifact.CodeImportsOut
			if err := deps.Artifact("code_imports", &codeImportsOut); err != nil {
				return nil, err
			}
			return artifact.CodeGraphIn{
				Repo:         deps.Repo(),
				Dependencies: codeImportsOut.PossibleDependencies,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "code_graph")
			var c3 codepipe.CodeGraph
			out, err := c3.Run(ctx, in.(artifact.CodeGraphIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.CodeGraphIn
				Salt string
			}{in.(artifact.CodeGraphIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["code_tasks"] = WorkerSpec{
		Key:         "code_tasks",
		Requires:    []string{"code_graph"},
		Description: "Chunk graph nodes into LLM-sized tasks with token estimates per file.",
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var graph artifact.CodeGraphOut
			if err := deps.Artifact("code_graph", &graph); err != nil {
				return nil, err
			}
			capPerChunk := 4096
			if deps.Env().LLM != nil && deps.Env().LLM.TokenCapacity() > 0 {
				capPerChunk = deps.Env().LLM.TokenCapacity()
			}
			return artifact.CodeTasksIn{
				Repo:        deps.Repo(),
				RepoFS:      deps.Env().RepoFS,
				Graph:       graph.Graph,
				CapPerChunk: capPerChunk,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "code_tasks")
			x := pipelineCodeTasks{LLM: env.LLM}
			out, err := x.Run(ctx, in.(artifact.CodeTasksIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.CodeTasksIn
				Salt string
			}{in.(artifact.CodeTasksIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["code_symbols"] = WorkerSpec{
		Key:         "code_symbols",
		Requires:    []string{"code_tasks"},
		Description: "LLM traverses tasks to build identifier reference maps (outgoing/incoming).",
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var codeTasksOut artifact.CodeTasksOut
			if err := deps.Artifact("code_tasks", &codeTasksOut); err != nil {
				return nil, err
			}
			return artifact.CodeSymbolsIn{
				Repo:   deps.Repo(),
				RepoFS: deps.Env().RepoFS,
				Tasks:  codeTasksOut,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "code_symbols")
			x := pipelineCodeSymbols{LLM: env.LLM}
			out, err := x.Run(ctx, in.(artifact.CodeSymbolsIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.CodeSymbolsIn
				Salt string
			}{in.(artifact.CodeSymbolsIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}

// thin adapters
type pipelineCodeRoots struct {
	LLM   llmclient.LLMClient
	Tools llmtool.ToolProvider
}
type pipelineCodeSpecs struct{ LLM llmclient.LLMClient }
type pipelineCodeImports struct{}
type pipelineCodeGraph struct{}
type pipelineCodeTasks struct{ LLM llmclient.LLMClient }
type pipelineCodeSymbols struct{ LLM llmclient.LLMClient }

func (p pipelineCodeRoots) Run(ctx context.Context, in artifact.CodeRootsIn) (artifact.CodeRootsOut, error) {
	real := codepipe.CodeRoots{LLM: p.LLM, Tools: p.Tools}
	return real.Run(ctx, in)
}
func (p pipelineCodeSpecs) Run(ctx context.Context, in artifact.CodeSpecsIn) (artifact.CodeSpecsOut, error) {
	real := codepipe.CodeSpecs{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineCodeImports) Run(ctx context.Context, in artifact.CodeImportsIn) (artifact.CodeImportsOut, error) {
	real := codepipe.CodeImports{}
	return real.Run(ctx, in)
}
func (pipelineCodeGraph) Run(ctx context.Context, in artifact.CodeGraphIn) (artifact.CodeGraphOut, error) {
	real := codepipe.CodeGraph{}
	return real.Run(ctx, in)
}
func (p pipelineCodeTasks) Run(ctx context.Context, in artifact.CodeTasksIn) (artifact.CodeTasksOut, error) {
	real := codepipe.CodeTasks{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (p pipelineCodeSymbols) Run(ctx context.Context, in artifact.CodeSymbolsIn) (artifact.CodeSymbolsOut, error) {
	real := codepipe.CodeSymbols{LLM: p.LLM}
	return real.Run(ctx, in)
}
