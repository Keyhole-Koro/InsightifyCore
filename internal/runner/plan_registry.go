package runner

import (
	"context"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	planPipe "insightify/internal/pipeline/plan"
)

func BuildRegistryPlanDependencies(env *Env) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}

	reg["worker_DAG"] = WorkerSpec{
		Key:         "worker_DAG",
		Description: "Generates an execution plan based on the provided graph spec.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			var workers []artifact.WorkerMeta
			if resolver := deps.Env().Resolver; resolver != nil {
				for _, spec := range resolver.List() {
					workers = append(workers, artifact.WorkerMeta{
						Key:         spec.Key,
						Description: spec.Description,
						Requires:    spec.Requires,
					})
				}
			}
			return artifact.PlanDependenciesIn{
				RepoPath: deps.Root(),
				Workers:  workers,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "worker_DAG")
			p := planPipe.PlanContext{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.PlanDependenciesIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: out.ClientView}, nil
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
