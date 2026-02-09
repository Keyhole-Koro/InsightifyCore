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

	reg["plan_source_scout"] = WorkerSpec{
		Key:         "plan_source_scout",
		Description: "Extracts or recommends a GitHub repository URL from user intent.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelMiddle,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			input := strings.TrimSpace(deps.Env().InitPurposeUserInput)
			isBootstrap := deps.Env().InitPurposeBootstrap
			if input == "" && !isBootstrap {
				isBootstrap = true
			}
			return artifact.PlanSourceScoutIn{
				UserInput:   input,
				IsBootstrap: isBootstrap,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "plan_source_scout")
			p := plan.SourceScout{LLM: env.LLM}
			out, err := p.Run(ctx, in.(artifact.PlanSourceScoutIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{RuntimeState: out, ClientView: nil}, nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   artifact.PlanSourceScoutIn
				Salt string
			}{in.(artifact.PlanSourceScoutIn), env.ModelSalt})
		},
		Strategy: jsonStrategy{},
	}

	// plan_pipeline: interactive bootstrap flow
	reg["plan_pipeline"] = WorkerSpec{
		Key:         "plan_pipeline",
		Requires:    []string{"plan_source_scout"},
		Description: "Interactive planning pipeline: collects intent, confirms understanding, generates plan.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelLow,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			input := strings.TrimSpace(deps.Env().InitPurposeUserInput)
			isBootstrap := deps.Env().InitPurposeBootstrap
			if input == "" && !isBootstrap {
				isBootstrap = true
			}
			var scout artifact.PlanSourceScoutOut
			if err := deps.Artifact("plan_source_scout", &scout); err != nil {
				return nil, err
			}
			return plan.BootstrapIn{
				UserInput:   input,
				IsBootstrap: isBootstrap,
				Scout:       scout,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "plan_pipeline")
			p := plan.BootstrapPipeline{
				LLM:     env.LLM,
				Emitter: EmitterFrom(ctx), // runner.RunEventEmitter implements ChunkEmitter
			}
			out, err := p.Run(ctx, in.(plan.BootstrapIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			// Update env with collected purpose/repo
			if purpose := strings.TrimSpace(out.Result.Purpose); purpose != "" {
				env.InitPurpose = purpose
			}
			if repoURL := strings.TrimSpace(out.Result.RepoURL); repoURL != "" {
				env.InitPurposeRepoURL = repoURL
			}
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

			planPipeDesc := "Interactive planning pipeline: collects intent, confirms understanding, generates plan."
			if purpose := strings.TrimSpace(deps.Env().InitPurpose); purpose != "" {
				planPipeDesc = purpose
			}
			workersByKey["plan_pipeline"] = artifact.WorkerMeta{
				Key:         "plan_pipeline",
				Description: planPipeDesc,
			}

			workerDAG := workersByKey["worker_DAG"]
			workerDAG.Key = "worker_DAG"
			if strings.TrimSpace(workerDAG.Description) == "" {
				workerDAG.Description = "Generates an execution plan based on the provided graph spec."
			}
			if !containsWorkerKey(workerDAG.Requires, "plan_pipeline") {
				workerDAG.Requires = append(workerDAG.Requires, "plan_pipeline")
			}
			workersByKey["worker_DAG"] = workerDAG

			workers := make([]artifact.WorkerMeta, 0, len(workersByKey))
			for _, w := range workersByKey {
				workers = append(workers, w)
			}

			return artifact.PlanDependenciesIn{
				RepoPath:    deps.Root(),
				InitPurpose: strings.TrimSpace(deps.Env().InitPurpose),
				InitRepoURL: strings.TrimSpace(deps.Env().InitPurposeRepoURL),
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
