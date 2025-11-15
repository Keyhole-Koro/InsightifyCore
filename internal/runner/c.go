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
	}

	return reg
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
