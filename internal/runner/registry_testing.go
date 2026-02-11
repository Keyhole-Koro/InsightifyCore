package runner

import (
	"context"
	"strings"

	"insightify/internal/artifact"
	"insightify/internal/llm"
	"insightify/internal/workers/plan"
)

// BuildRegistryTest defines test-only workers used for interactive/manual validation.
func BuildRegistryTest(env *Env) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}

	reg["testllmChar"] = WorkerSpec{
		Key:         "testllmChar",
		Description: "Interactive chat test worker for frontend validation.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelLow,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			ic := deps.Env().InitCtx
			input := strings.TrimSpace(ic.UserInput)
			isBootstrap := ic.Bootstrap
			if input == "" && !isBootstrap {
				isBootstrap = true
			}
			return plan.BootstrapIn{
				UserInput:   input,
				IsBootstrap: isBootstrap,
				Scout:       artifact.PlanSourceScoutOut{},
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			// Reuse init_purpose worker context for deterministic fake-LLM behavior in local testing.
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

	return reg
}
