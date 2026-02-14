package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"insightify/internal/workers/plan"
)

// ExecuteWorker runs a single worker by key using the resolver in env.
// It centralizes input construction, dependency checks, and cache strategy handling.
func ExecuteWorker(ctx context.Context, runtime Runtime, workerID string, params map[string]string) (WorkerOutput, error) {
	if runtime == nil || runtime.GetResolver() == nil {
		return WorkerOutput{}, fmt.Errorf("run environment resolver is not available")
	}

	spec, ok := runtime.GetResolver().Get(workerID)
	if !ok {
		return WorkerOutput{}, fmt.Errorf("unknown worker_id: %s", workerID)
	}

	deps := newDeps(runtime, spec.Key, spec.Requires)
	var (
		input any
		err   error
	)
	if spec.BuildInput != nil {
		input, err = spec.BuildInput(ctx, deps)
		if err != nil {
			return WorkerOutput{}, fmt.Errorf("build input failed: %w", err)
		}
	}
	input = applyRunParams(input, params)

	if err := verifyDepsUsage(runtime, spec.Key, deps); err != nil {
		return WorkerOutput{}, err
	}
	if spec.Run == nil {
		return WorkerOutput{}, fmt.Errorf("worker %q has no run function", workerID)
	}

	inputFP := ""
	if spec.Fingerprint != nil {
		inputFP = spec.Fingerprint(input, runtime)
	} else {
		inputFP = JSONFingerprint(input)
	}

	strategy := spec.Strategy
	if strategy == nil {
		strategy = JSONStrategy()
	}
	if out, ok := strategy.TryLoad(ctx, spec, runtime, inputFP); ok {
		return out, nil
	}

	out, err := spec.Run(ctx, input, runtime)
	if err != nil {
		return WorkerOutput{}, err
	}
	if err := strategy.Save(ctx, spec, runtime, out, inputFP); err != nil {
		return WorkerOutput{}, fmt.Errorf("save worker output failed: %w", err)
	}
	return out, nil
}

func verifyDepsUsage(runtime Runtime, workerKey string, deps *depsImpl) error {
	if runtime == nil || deps == nil {
		return nil
	}

	unused := deps.verifyUsage()
	if len(unused) == 0 {
		return nil
	}
	msg := fmt.Sprintf("worker %q declared unused requires: %s", workerKey, strings.Join(unused, ", "))
	switch runtime.GetDepsUsage() {
	case DepsUsageIgnore:
		return nil
	case DepsUsageWarn:
		log.Printf("WARN: %s", msg)
		return nil
	default:
		return errors.New(msg)
	}
}

func applyRunParams(input any, params map[string]string) any {
	if input == nil {
		return nil
	}
	if len(params) == 0 {
		return input
	}

	switch in := input.(type) {
	case map[string]any:
		for k, v := range params {
			in[k] = v
		}
		return in
	case plan.BootstrapIn:
		if v := strings.TrimSpace(params["input"]); v != "" {
			in.UserInput = v
		}
		return in
	default:
		return input
	}
}
