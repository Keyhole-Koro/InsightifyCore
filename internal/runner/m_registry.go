package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	mlpipe "insightify/internal/pipeline/mainline"
)

// BuildRegistryMainline defines m0/m1 in one place.
// Add/modify phases here without touching main or execution logic.
func BuildRegistryMainline(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}
	reg["m0"] = PhaseSpec{
		Key:         "m0",
		File:        "m0.json",
		Description: "Scan repo layout and ask LLM to classify main source roots, library/vendor roots, and config hotspots.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			return artifact.M0In{Repo: deps.Repo()}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "m0")
			p := mlpipe.M0{LLM: env.LLM, Tools: env.MCP}
			out, err := p.Run(ctx, in.(artifact.M0In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.M0In
				Salt string
			}{in.(artifact.M0In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["m1"] = PhaseSpec{
		Key:         "m1",
		File:        "m1.json",
		Requires:    []string{"m0"},
		Description: "LLM drafts initial architecture hypothesis from file index + Markdown docs and proposes next files to open.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var m0prev artifact.M0Out
			if err := deps.Artifact("m0", &m0prev); err != nil {
				return nil, err
			}
			return artifact.M1In{
				Repo:         deps.Repo(),
				LibraryRoots: m0prev.LibraryRoots,
				Hints:        &artifact.M1Hints{},
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "m1")
			p := mlpipe.M1{LLM: env.LLM, Tools: env.MCP}
			out, err := p.Run(ctx, in.(artifact.M1In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.M1In
				Salt string
			}{in.(artifact.M1In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}
