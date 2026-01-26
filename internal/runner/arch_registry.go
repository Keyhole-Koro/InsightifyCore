package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	mlpipe "insightify/internal/pipeline/architecture"
)

// BuildRegistryMainline defines arch_design.
// Add/modify phases here without touching main or execution logic.
func BuildRegistryMainline(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}

	reg["arch_design"] = PhaseSpec{
		Key:         "arch_design",
		File:        "arch_design.json",
		Requires:    []string{"code_roots"},
		Description: "LLM drafts initial architecture hypothesis from file index + Markdown docs and proposes next files to open.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var c0prev artifact.CodeRootsOut
			if err := deps.Artifact("code_roots", &c0prev); err != nil {
				return nil, err
			}
			return artifact.ArchDesignIn{
				Repo:         deps.Repo(),
				LibraryRoots: c0prev.LibraryRoots,
				Hints:        &artifact.ArchDesignHints{},
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "arch_design")
			p := mlpipe.ArchDesign{LLM: env.LLM, Tools: env.MCP}
			out, err := p.Run(ctx, in.(artifact.ArchDesignIn))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.ArchDesignIn
				Salt string
			}{in.(artifact.ArchDesignIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}
