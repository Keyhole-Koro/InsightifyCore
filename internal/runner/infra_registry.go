package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	extpipe "insightify/internal/pipeline/external"
)

// BuildRegistryExternal wires the external (x*) pipeline stages.
func BuildRegistryExternal(env *Env) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}

	reg["infra_context"] = WorkerSpec{
		Key:         "infra_context",
		File:        "infra_context.json",
		Requires:    []string{"arch_design", "code_symbols", "code_roots"}, // Explicit code_roots dependency added for roots
		Description: "LLM summarizes external systems/infra using architecture (arch_design) + identifier refs (code_symbols), surfacing evidence gaps.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var c0 artifact.CodeRootsOut
			if err := deps.Artifact("code_roots", &c0); err != nil {
				return nil, err
			}
			var m1 artifact.ArchDesignOut
			if err := deps.Artifact("arch_design", &m1); err != nil {
				return nil, err
			}
			var c5 artifact.CodeSymbolsOut
			if err := deps.Artifact("code_symbols", &c5); err != nil {
				return nil, err
			}
			samples := collectInfraSamples(deps.Env().RepoFS, deps.Repo(), c0, 16, 16000)
			summaries := selectIdentifierSummaries(c5.Files, deps.Repo(), c0, 40)
			return artifact.InfraContextIn{
				Repo:                deps.Repo(),
				Roots:               c0,
				Architecture:        m1,
				ConfigSamples:       samples,
				IdentifierSummaries: summaries,
				ConfidenceThreshold: 0.65,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "infra_context")
			p := extpipe.InfraContext{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.InfraContextIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.InfraContextIn
				Salt string
			}{in.(artifact.InfraContextIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	reg["infra_refine"] = WorkerSpec{
		Key:         "infra_refine",
		File:        "infra_refine.json",
		Requires:    []string{"infra_context"},
		Description: "LLM drills into evidence gaps from infra_context by opening targeted files/snippets.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var prev artifact.InfraContextOut
			if err := deps.Artifact("infra_context", &prev); err != nil {
				return nil, err
			}
			files := collectGapFiles(deps.Env().RepoFS, deps.Repo(), prev.EvidenceGaps, 24, 64000)
			return artifact.InfraRefineIn{
				Repo:     deps.Repo(),
				Previous: prev,
				Files:    files,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "infra_refine")
			p := extpipe.InfraRefine{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.InfraRefineIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.InfraRefineIn
				Salt string
			}{in.(artifact.InfraRefineIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	return reg
}
