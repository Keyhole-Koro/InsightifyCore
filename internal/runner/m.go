package runner

import (
	"context"

	"insightify/internal/llm"
	mlpipe "insightify/internal/pipeline/mainline"
	ml "insightify/internal/types/mainline"
)

// BuildRegistryMainline defines m0/m1/m2 in one place.
// Add/modify phases here without touching main or execution logic.
func BuildRegistryMainline(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}
	reg["m0"] = PhaseSpec{
		Key:  "m0",
		File: "m0.json",
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			return ml.M0In{Repo: env.Repo}, nil
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
	}

	reg["m1"] = PhaseSpec{
		Key:      "m1",
		File:     "m1.json",
		Requires: []string{"m0"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			m0prev, err := Artifact[ml.M0Out](env, "m0")
			if err != nil {
				return nil, err
			}
			ig := UniqueStrings(baseNames(m0prev.LibraryRoots...)...)
			return ml.M1In{
				Repo:       env.Repo,
				IgnoreDirs: ig,
				Hints:      &ml.M1Hints{},
				Limits:     &ml.M1Limits{MaxNext: env.MaxNext},
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
	}

	reg["m2"] = PhaseSpec{
		Key:      "m2",
		File:     "m2.json",
		Requires: []string{"m1"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			m1 := MustArtifact[ml.M1Out](env, "m1")

			m0prev, err := Artifact[ml.M0Out](env, "m0")
			if err != nil {
				return nil, err
			}

			return ml.M2In{
				Repo:         env.Repo,
				RepoRoot:     env.RepoRoot,
				Roots:        &m0prev,
				Previous:     m1,
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
	}

	return reg
}
