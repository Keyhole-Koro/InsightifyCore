package runner

import (
	"context"

	"insightify/internal/llm"
	extpipe "insightify/internal/pipeline/external"
	cb "insightify/internal/types/codebase"
	ex "insightify/internal/types/external"
	ml "insightify/internal/types/mainline"
)

// BuildRegistryExternal wires the external (x*) pipeline stages.
func BuildRegistryExternal(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}

	reg["x0"] = PhaseSpec{
		Key:         "x0",
		File:        "x0.json",
		Requires:    []string{"m1", "c4"},
		Description: "LLM summarizes external systems/infra using architecture (m1) + identifier refs (c4), surfacing evidence gaps.",
		Consumes:    []string{"architecture_hypothesis", "references", "layout_roots"},
		Produces:    []string{"external_overview", "evidence_gaps"},
		UsesLLM:     true,
		Tags:        []string{"external", "infra"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			m0, err := Artifact[ml.M0Out](env, "m0")
			if err != nil {
				return nil, err
			}
			m1, err := Artifact[ml.M1Out](env, "m1")
			if err != nil {
				return nil, err
			}
			c4, err := Artifact[cb.C4Out](env, "c4")
			if err != nil {
				return nil, err
			}
			samples := collectInfraSamples(env.RepoFS, env.RepoRoot, m0, 16, 16000)
			summaries := selectIdentifierSummaries(c4.Files, env.RepoRoot, m0, 40)
			return ex.X0In{
				Repo:                env.Repo,
				Roots:               m0,
				Architecture:        m1,
				ConfigSamples:       samples,
				IdentifierSummaries: summaries,
				ConfidenceThreshold: 0.65,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "x0")
			p := extpipe.X0{LLM: env.LLM}
			out, err := p.Run(ctx, in.(ex.X0In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   ex.X0In
				Salt string
			}{in.(ex.X0In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["x1"] = PhaseSpec{
		Key:         "x1",
		File:        "x1.json",
		Requires:    []string{"x0"},
		Description: "LLM drills into evidence gaps from x0 by opening targeted files/snippets.",
		Consumes:    []string{"external_overview", "evidence_gaps"},
		Produces:    []string{"external_verification"},
		UsesLLM:     true,
		Tags:        []string{"external", "infra"},
		BuildInput: func(ctx context.Context, env *Env) (any, error) {
			prev, err := Artifact[ex.X0Out](env, "x0")
			if err != nil {
				return nil, err
			}
			files := collectGapFiles(env.RepoFS, env.RepoRoot, prev.EvidenceGaps, 24, 64000)
			return ex.X1In{
				Repo:     env.Repo,
				Previous: prev,
				Files:    files,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "x1")
			p := extpipe.X1{LLM: env.LLM}
			out, err := p.Run(ctx, in.(ex.X1In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   ex.X1In
				Salt string
			}{in.(ex.X1In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}
