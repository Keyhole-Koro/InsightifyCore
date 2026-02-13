package runner

import (
	"context"

	"insightify/internal/llm"
	"insightify/internal/workers/plan"
	testpipe "insightify/internal/workers/testworker"
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
			input := UserInputFromContext(ctx)
			return plan.BootstrapIn{
				UserInput: input,
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
			return bootstrapWorkerOutput(out), nil
		},
		Fingerprint: func(in any, env *Env) string {
			return JSONFingerprint(struct {
				In   plan.BootstrapIn
				Salt string
			}{in.(plan.BootstrapIn), env.ModelSalt})
		},
		Strategy: versionedStrategy{},
	}

	reg["testllmChatNode"] = WorkerSpec{
		Key:         "testllmChatNode",
		Description: "Casual conversation worker that renders an LLM chat node for frontend interaction testing.",
		LLMRole:     llm.ModelRoleWorker,
		LLMLevel:    llm.ModelLevelLow,
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			input := UserInputFromContext(ctx)
			return plan.BootstrapIn{
				UserInput: input,
			}, nil
		},
		Run: func(ctx context.Context, in any, env *Env) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "testllmChatNode")
			p := testpipe.LLMChatNodePipeline{
				LLM:     env.LLM,
				Emitter: EmitterFrom(ctx),
			}
			out, err := p.Run(ctx, in.(plan.BootstrapIn))
			if err != nil {
				return WorkerOutput{}, err
			}
			return bootstrapWorkerOutput(out), nil
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
