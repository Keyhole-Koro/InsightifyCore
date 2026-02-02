package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	planPipe "insightify/internal/pipeline/plan"
)

func BuildRegistryPlanDependencies(env *Env) map[string]PhaseSpec {
	reg := map[string]PhaseSpec{}

	reg["phase_DAG"] = PhaseSpec{
		Key:         "phase_DAG",
		File:        "phase_DAG.json",
		Description: "Generates an execution plan based on the provided graph spec.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var phases []artifact.PhaseMeta
			if resolver := deps.Env().Resolver; resolver != nil {
				for _, spec := range resolver.List() {
					phases = append(phases, artifact.PhaseMeta{
						Key:         spec.Key,
						Description: spec.Description,
						Requires:    spec.Requires,
					})
				}
			}
			return artifact.PlanDependenciesIn{
				RepoPath: deps.Root(),
				Phases:   phases,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (PhaseOutput, error) {
			ctx = llm.WithPhase(ctx, "phase_DAG")
			p := planPipe.PlanContext{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.PlanDependenciesIn))
			if err != nil {
				return PhaseOutput{}, err
			}
			return PhaseOutput{RuntimeState: out, ClientView: out.ClientView}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.PlanDependenciesIn
				Salt string
			}{in.(artifact.PlanDependenciesIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}
	return reg
}
