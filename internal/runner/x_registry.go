package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	extpipe "insightify/internal/pipeline/external"
)

// BuildRegistryExternal wires the external (x*) pipeline stages.
func BuildRegistryExternal(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}

	reg["x0"] = PhaseSpec{
		Key:         "x0",
		File:        "x0.json",
		Requires:    []string{"m1", "c4", "m0"}, // Explicit m0 dependency added for roots
		Description: "LLM summarizes external systems/infra using architecture (m1) + identifier refs (c4), surfacing evidence gaps.",
		Consumes:    []string{"architecture_hypothesis", "references", "layout_roots"},
		Produces:    []string{"external_overview", "evidence_gaps"},
		UsesLLM:     true,
		Tags:        []string{"external", "infra"},
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var m0 artifact.M0Out
			if err := deps.Artifact("m0", &m0); err != nil {
				return nil, err
			}
			var m1 artifact.M1Out
			if err := deps.Artifact("m1", &m1); err != nil {
				return nil, err
			}
			var c4 artifact.C4Out
			if err := deps.Artifact("c4", &c4); err != nil {
				return nil, err
			}
			samples := collectInfraSamples(deps.Env().RepoFS, deps.Repo(), m0, 16, 16000)
			summaries := selectIdentifierSummaries(c4.Files, deps.Repo(), m0, 40)
			return artifact.X0In{
				Repo:                deps.Repo(),
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
			out, err := p.Run(ctx, in.(artifact.X0In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.X0In
				Salt string
			}{in.(artifact.X0In), env.ModelSalt})
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
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var prev artifact.X0Out
			if err := deps.Artifact("x0", &prev); err != nil {
				return nil, err
			}
			files := collectGapFiles(deps.Env().RepoFS, deps.Repo(), prev.EvidenceGaps, 24, 64000)
			return artifact.X1In{
				Repo:     deps.Repo(),
				Previous: prev,
				Files:    files,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "x1")
			p := extpipe.X1{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.X1In))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.X1In
				Salt string
			}{in.(artifact.X1In), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}
