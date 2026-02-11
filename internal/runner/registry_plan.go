package runner

import (
	"context"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	"insightify/internal/workers/plan"
)

// BuildRegistryPlan builds workers for the plan pipeline.
func BuildRegistryPlan(env *Env) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}

	// bootstrap: preferred key for interactive bootstrap flow.
	// Keep init_purpose as a compatibility alias.
	reg["bootstrap"] = WorkerSpec{
		Key:         "bootstrap",
		Description: "Interactive intent bootstrap worker: collects user intent and repository context.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelLow,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			ic := deps.Env().InitCtx
			input := strings.TrimSpace(ic.UserInput)
			return plan.BootstrapIn{
				UserInput: input,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "bootstrap")
			p := plan.BootstrapPipeline{
				LLM:     env.LLM,
				Emitter: EmitterFrom(ctx),
			}
			out, err := p.Run(ctx, in.(plan.BootstrapIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			env.InitCtx.SetPurpose(out.Result.Purpose, out.Result.RepoURL)
			return WorkerOutput{RuntimeState: out, ClientView: out.ClientView}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   plan.BootstrapIn
				Salt string
			}{in.(plan.BootstrapIn), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	// init_purpose legacy alias kept for compatibility.
	reg["init_purpose"] = WorkerSpec{
		Key:         "init_purpose",
		Description: "Legacy alias of bootstrap.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelLow,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			ic := deps.Env().InitCtx
			input := strings.TrimSpace(ic.UserInput)
			return plan.BootstrapIn{
				UserInput: input,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "bootstrap")
			p := plan.BootstrapPipeline{
				LLM:     env.LLM,
				Emitter: EmitterFrom(ctx),
			}
			out, err := p.Run(ctx, in.(plan.BootstrapIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			env.InitCtx.SetPurpose(out.Result.Purpose, out.Result.RepoURL)
			return WorkerOutput{RuntimeState: out, ClientView: out.ClientView}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   plan.BootstrapIn
				Salt string
			}{in.(plan.BootstrapIn), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	// plan_pipeline legacy alias kept for compatibility. It delegates to init_purpose behavior.
	reg["plan_pipeline"] = WorkerSpec{
		Key:         "plan_pipeline",
		Description: "Legacy alias of init_purpose.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelLow,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			ic := deps.Env().InitCtx
			input := strings.TrimSpace(ic.UserInput)
			return plan.BootstrapIn{
				UserInput: input,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "init_purpose")
			p := plan.BootstrapPipeline{
				LLM:     env.LLM,
				Emitter: EmitterFrom(ctx),
			}
			out, err := p.Run(ctx, in.(plan.BootstrapIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			env.InitCtx.SetPurpose(out.Result.Purpose, out.Result.RepoURL)
			return WorkerOutput{RuntimeState: out, ClientView: out.ClientView}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   plan.BootstrapIn
				Salt string
			}{in.(plan.BootstrapIn), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	reg["worker_DAG"] = WorkerSpec{
		Key:         "worker_DAG",
		Description: "Generates an execution plan based on the provided graph spec.",
		LLMRole:     llm.ModelRolePlanner,
		LLMLevel:    llm.ModelLevelHigh,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			workersByKey := map[string]artifact.WorkerMeta{}
			if resolver := deps.Env().Resolver; resolver != nil {
				for _, spec := range resolver.List() {
					workersByKey[spec.Key] = artifact.WorkerMeta{
						Key:         spec.Key,
						Description: spec.Description,
						Requires:    spec.Requires,
					}
				}
			}

			bootstrapDesc := "Interactive intent bootstrap worker: collects user intent and repository context."
			if purpose := strings.TrimSpace(deps.Env().InitCtx.Purpose); purpose != "" {
				bootstrapDesc = purpose
			}
			workersByKey["bootstrap"] = artifact.WorkerMeta{
				Key:         "bootstrap",
				Description: bootstrapDesc,
			}

			workerDAG := workersByKey["worker_DAG"]
			workerDAG.Key = "worker_DAG"
			if strings.TrimSpace(workerDAG.Description) == "" {
				workerDAG.Description = "Generates an execution plan based on the provided graph spec."
			}
			if !containsWorkerKey(workerDAG.Requires, "bootstrap") {
				workerDAG.Requires = append(workerDAG.Requires, "bootstrap")
			}
			workersByKey["worker_DAG"] = workerDAG

			workers := make([]artifact.WorkerMeta, 0, len(workersByKey))
			for _, w := range workersByKey {
				workers = append(workers, w)
			}

			ic := deps.Env().InitCtx
			return artifact.PlanDependenciesIn{
				RepoPath:    deps.Root(),
				InitPurpose: strings.TrimSpace(ic.Purpose),
				InitRepoURL: strings.TrimSpace(ic.RepoURL),
				Workers:     workers,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "worker_DAG")
			p := plan.PlanContext{LLM: env.LLM}
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

func containsWorkerKey(keys []string, want string) bool {
	for _, k := range keys {
		if strings.TrimSpace(k) == want {
			return true
		}
	}
	return false
}
