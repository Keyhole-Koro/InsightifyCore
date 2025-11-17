package runner

import (
	"context"

	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	codepipe "insightify/internal/pipeline/codebase"

	cb "insightify/internal/types/codebase"
	ml "insightify/internal/types/mainline"
)

// BuildRegistryCodebase defines c0/c1 using the same registry/strategy system.
// c0 uses versionedStrategy; c1 uses jsonStrategy.
func BuildRegistryCodebase(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}
	reg["c0"] = PhaseSpec{
		Key:      "c0",
		File:     "c0.json", // latest pointer; versioned writes also occur
		Requires: []string{"m0"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			m0prev, err := Artifact[ml.M0Out](env, "m0")
			if err != nil {
				return nil, err
			}
			in := cb.C0In{
				Repo:  env.Repo,
				Roots: m0prev,
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
	}

	reg["c1"] = PhaseSpec{
		Key:      "c1",
		File:     "c1.json",
		Requires: []string{"c0", "m0"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			c0prev, err := Artifact[cb.C0Out](env, "c0")
			if err != nil {
				return nil, err
			}
			m0prev, err := Artifact[ml.M0Out](env, "m0")
			if err != nil {
				return nil, err
			}
			in := cb.C1In{
				Repo:     env.Repo,
				Families: c0prev.Families,
				Roots:    m0prev,
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
	}

	reg["c2"] = PhaseSpec{
		Key:      "c2",
		File:     "c2.json",
		Requires: []string{"c1"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			c1out, err := Artifact[cb.C1Out](env, "c1")
			if err != nil {
				return nil, err
			}
			return cb.C2In{
				Repo:         env.Repo,
				Dependencies: c1out.PossibleDependencies,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (any, error) {
			ctx = llm.WithPhase(ctx, "c2")
			var c2 codepipe.C2
			return c2.Run(ctx, in.(cb.C2In))
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   cb.C2In
				Salt string
			}{in.(cb.C2In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["c3"] = PhaseSpec{
		Key:      "c3",
		File:     "c3.json",
		Requires: []string{"c2"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			graph, err := Artifact[cb.C2Out](env, "c2")
			if err != nil {
				return nil, err
			}
			capPerChunk := 4096
			if env.LLM != nil && env.LLM.TokenCapacity() > 0 {
				capPerChunk = env.LLM.TokenCapacity()
			}
			return cb.C3In{
				Repo:        env.Repo,
				RepoFS:      env.RepoFS,
				Graph:       graph.Graph,
				CapPerChunk: capPerChunk,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (any, error) {
			ctx = llm.WithPhase(ctx, "c3")
			x := pipelineC3{LLM: env.LLM}
			return x.Run(ctx, in.(cb.C3In))
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   cb.C3In
				Salt string
			}{in.(cb.C3In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["c4"] = PhaseSpec{
		Key:      "c4",
		File:     "c4.json",
		Requires: []string{"c3"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			c3out, err := Artifact[cb.C3Out](env, "c3")
			if err != nil {
				return nil, err
			}
			return cb.C4In{
				Repo:   env.Repo,
				RepoFS: env.RepoFS,
				Tasks:  c3out,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (any, error) {
			ctx = llm.WithPhase(ctx, "c4")
			x := pipelineC4{LLM: env.LLM}
			return x.Run(ctx, in.(cb.C4In))
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   cb.C4In
				Salt string
			}{in.(cb.C4In), env.ModelSalt})
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

func (p pipelineC0) Run(ctx context.Context, in cb.C0In) (cb.C0Out, error) {
	real := codepipe.C0{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (pipelineC1) Run(ctx context.Context, in cb.C1In) (cb.C1Out, error) {
	real := codepipe.C1{}
	return real.Run(ctx, in)
}
func (pipelineC2) Run(ctx context.Context, in cb.C2In) (cb.C2Out, error) {
	real := codepipe.C2{}
	return real.Run(ctx, in)
}
func (p pipelineC3) Run(ctx context.Context, in cb.C3In) (cb.C3Out, error) {
	real := codepipe.C3{LLM: p.LLM}
	return real.Run(ctx, in)
}
func (p pipelineC4) Run(ctx context.Context, in cb.C4In) (cb.C4Out, error) {
	real := codepipe.C4{LLM: p.LLM}
	return real.Run(ctx, in)
}
