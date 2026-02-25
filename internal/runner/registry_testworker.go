package runner

import (
	"context"
	"fmt"
	"insightify/internal/llm/middleware"
	"insightify/internal/workers/plan"
	testpipe "insightify/internal/workers/testworker"
)

type interactionAdapter struct {
	runID  string
	nodeID string
	waiter InteractionWaiter
}

func (a *interactionAdapter) WaitForInput(ctx context.Context) (string, error) {
	if a == nil || a.waiter == nil {
		return "", context.Canceled
	}
	return a.waiter.WaitForInput(ctx, a.runID, a.nodeID)
}

func (a *interactionAdapter) PublishOutput(ctx context.Context, message string) error {
	if a == nil || a.waiter == nil {
		return context.Canceled
	}
	return a.waiter.PublishOutput(ctx, a.runID, a.nodeID, "", message)
}

// BuildRegistryTestWorker wires test-only workers used for interaction prototyping.
func init() {
	RegisterBuilder(BuildRegistryTestWorker)
}

func BuildRegistryTestWorker(_ Runtime) map[string]WorkerSpec {
	reg := map[string]WorkerSpec{}

	reg["testllmChatNode"] = WorkerSpec{
		Key:         "testllmChatNode",
		Description: "Test LLM chat node worker for interactive daily conversation loop.",
		BuildInput: func(ctx context.Context, deps Deps) (any, error) {
			return plan.BootstrapIn{}, nil
		},
		Run: func(ctx context.Context, in any, runtime Runtime) (WorkerOutput, error) {
			ctx = llm.WithWorker(ctx, "testllmChatNode")
			p := testpipe.LLMChatNodePipeline{
				LLM: runtime.GetLLM(),
			}
			if runID, runOK := RunIDFromContext(ctx); runOK {
				if waiter, waiterOK := InteractionWaiterFromContext(ctx); waiterOK {
					nodeID, nodeOK := NodeIDFromContext(ctx)
					if !nodeOK || nodeID == "" {
						return WorkerOutput{}, fmt.Errorf("testllmChatNode requires node_id in context")
					}
					p.Interaction = &interactionAdapter{
						runID:  runID,
						nodeID: nodeID,
						waiter: waiter,
					}
				}
			}
			if err := p.Run(ctx, in.(plan.BootstrapIn)); err != nil {
				return WorkerOutput{}, err
			}
			return WorkerOutput{
				RuntimeState: map[string]any{"status": "completed"},
				ClientView:   nil,
			}, nil
		},
		Fingerprint: func(in any, runtime Runtime) string {
			return JSONFingerprint(struct {
				In   plan.BootstrapIn
				Salt string
			}{in.(plan.BootstrapIn), runtime.GetModelSalt()})
		},
		Strategy: VersionedStrategy(),
	}

	return reg
}
