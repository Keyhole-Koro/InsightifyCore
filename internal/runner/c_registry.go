package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	codepipe "insightify/internal/pipeline/codebase"
)

// BuildRegistryCodebase defines c0/c1 using the same registry/strategy system.
// c0 uses versionedStrategy; c1 uses jsonStrategy.
func BuildRegistryCodebase(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}
	reg["c0"] = PhaseSpec{
		Key:         "c0",
		File:        "c0.json", // latest pointer; versioned writes also occur
		Requires:    []string{"m0"},
		Description: "LLM infers language families/import heuristics from extension counts and roots.",
		Consumes:    []string{"layout_roots", "ext_counts"},
		Produces:    []string{"language_families"},
		UsesLLM:     true,
		Tags:        []string{"codebase", "language-detection"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var m0prev artifact.M0Out
			if err := deps.Artifact("m0", &m0prev); err != nil {
				return nil, err
			}
			in := artifact.C0In{
				Repo:  deps.Repo(),
				Roots: m0prev,
			}
			return in, nil
		},

		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "c0")
			x := pipelineC0{LLM: env.LLM}
			out, err := x.Run(ctx, in.(artifact.C0In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			// Even though versioned, keep a meta fingerprint for traceability.
			return JSONFingerprint(struct {
				In   artifact.C0In
				Salt string
			}{in.(artifact.C0In), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	reg["c1"] = PhaseSpec{
		Key:         "c1",
		File:        "c1.json",
		Requires:    []string{"c0", "m0"},
		Description: "Word-index dependency sweep across source roots to collect possible file-level dependencies.",
		Consumes:    []string{"language_families", "layout_roots", "file_index"},
		Produces:    []string{"raw_dependencies"},
		Tags:        []string{"codebase", "graph"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var c0prev artifact.C0Out
			if err := deps.Artifact("c0", &c0prev); err != nil {
				return nil, err
			}
			var m0prev artifact.M0Out
			if err := deps.Artifact("m0", &m0prev); err != nil {
				return nil, err
			}
			in := artifact.C1In{
				Repo:     deps.Repo(),
				Families: c0prev.Families,
				Roots:    m0prev,
			}
			return in, nil
		},

		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "c1")
			x := pipelineC1{}
			out, err := x.Run(ctx, in.(artifact.C1In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.C1In
				Salt string
			}{in.(artifact.C1In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["c2"] = PhaseSpec{
		Key:         "c2",
		File:        "c2.json",
		Requires:    []string{"c1"},
		Description: "Normalize dependency hits into a DAG and drop weaker bidirectional edges.",
		Consumes:    []string{"raw_dependencies"},
		Produces:    []string{"dependency_graph"},
		Tags:        []string{"codebase", "graph"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var c1out artifact.C1Out
			if err := deps.Artifact("c1", &c1out); err != nil {
				return nil, err
			}
			return artifact.C2In{
				Repo:         deps.Repo(),
				Dependencies: c1out.PossibleDependencies,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "c2")
			var c2 codepipe.C2
			out, err := c2.Run(ctx, in.(artifact.C2In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.C2In
				Salt string
			}{in.(artifact.C2In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["c3"] = PhaseSpec{
		Key:         "c3",
		File:        "c3.json",
		Requires:    []string{"c2"},
		Description: "Chunk graph nodes into LLM-sized tasks with token estimates per file.",
		Consumes:    []string{"dependency_graph"},
		Produces:    []string{"llm_task_graph"},
		UsesLLM:     true,
		Tags:        []string{"codebase", "chunking"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var graph artifact.C2Out
			if err := deps.Artifact("c2", &graph); err != nil {
				return nil, err
			}
			capPerChunk := 4096
			if deps.Env().LLM != nil && deps.Env().LLM.TokenCapacity() > 0 {
				capPerChunk = deps.Env().LLM.TokenCapacity()
			}
			return artifact.C3In{
				Repo:        deps.Repo(),
				RepoFS:      deps.Env().RepoFS,
				Graph:       graph.Graph,
				CapPerChunk: capPerChunk,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "c3")
			x := pipelineC3{LLM: env.LLM}
			out, err := x.Run(ctx, in.(artifact.C3In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.C3In
				Salt string
			}{in.(artifact.C3In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["c4"] = PhaseSpec{
		Key:         "c4",
		File:        "c4.json",
		Requires:    []string{"c3"},
		Description: "LLM traverses tasks to build identifier reference maps (outgoing/incoming).",
		Consumes:    []string{"llm_task_graph"},
		Produces:    []string{"references"},
		UsesLLM:     true,
		Tags:        []string{"codebase", "references"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var c3out artifact.C3Out
			if err := deps.Artifact("c3", &c3out); err != nil {
				return nil, err
			}
			return artifact.C4In{
				Repo:   deps.Repo(),
				RepoFS: deps.Env().RepoFS,
				Tasks:  c3out,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "c4")
			x := pipelineC4{LLM: env.LLM}
			out, err := x.Run(ctx, in.(artifact.C4In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.C4In
				Salt string
			}{in.(artifact.C4In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}

// thin adapters
type pipelineC0 struct{ LLM llmclient.LLMClient }
type pipelineC1 struct{}
type pipelineC2 struct{}
type pipelineC3 struct{ LLM llmclient.LLMClient }
type pipelineC4 struct{ LLM llmclient.LLMClient }

func (p pipelineC0) Run(ctx context.Context, in artifact.C0In) (artifact.C0Out, error) {
	real := codepipe.C0{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineC1) Run(ctx context.Context, in artifact.C1In) (artifact.C1Out, error) {
	real := codepipe.C1{}
	return real.Run(ctx, in)
}
func (pipelineC2) Run(ctx context.Context, in artifact.C2In) (artifact.C2Out, error) {
	real := codepipe.C2{}
	return real.Run(ctx, in)
}
func (p pipelineC3) Run(ctx context.Context, in artifact.C3In) (artifact.C3Out, error) {
	real := codepipe.C3{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (p pipelineC4) Run(ctx context.Context, in artifact.C4In) (artifact.C4Out, error) {
	real := codepipe.C4{LLM: p.LLM}
	return real.Run(ctx, in)
}
