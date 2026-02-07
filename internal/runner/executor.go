package runner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"insightify/internal/llm"
	"insightify/internal/utils"

	pipelinev1 "insightify/gen/go/pipeline/v1"
)

// ExecuteWorker builds input, applies force-from + strategy caching, runs, then invalidates downstream.
func ExecuteWorker(ctx context.Context, spec WorkerSpec, env *Env) error {
	_, err := ExecuteWorkerWithResult(ctx, spec, env)
	return err
}

// ExecuteWorkerWithResult is the same as ExecuteWorker but also returns WorkerOutput.
// RuntimeState is populated from the legacy Run() return; ClientView is nil unless a worker chooses to set it.
func ExecuteWorkerWithResult(ctx context.Context, spec WorkerSpec, env *Env) (WorkerOutput, error) {
	var zero WorkerOutput
	if len(spec.Requires) > 0 {
		visiting := make(map[string]bool)
		for _, r := range spec.Requires {
			if err := ensureArtifact(ctx, r, env, visiting); err != nil {
				return zero, err
			}
		}
	}

	// Prepare Deps for usage tracking
	deps := newDeps(env, spec.Key, spec.Requires)

	// Build logical input using Deps
	in, err := spec.BuildInput(ctx, deps)
	if err != nil {
		return zero, err
	}

	// Verify usage (optional warning for now)
	if unused := deps.verifyUsage(); len(unused) > 0 {
		switch env.DepsUsage {
		case DepsUsageIgnore:
			// no-op
		case DepsUsageWarn:
			log.Printf("WARNING: worker %s declared but did not use: %v", spec.Key, unused)
		default:
			return zero, fmt.Errorf("worker %s declared but did not use: %v", spec.Key, unused)
		}
	}

	// Compute fingerprint
	fp := spec.Fingerprint(in, env)

	// Try cache (if strategy supports it)
	if out, ok := spec.Strategy.TryLoad(ctx, spec, env, fp); ok {
		return out, nil
	}

	if spec.LLMLevel == "" {
		return zero, fmt.Errorf("worker %s: llm level must be specified", spec.Key)
	}

	// Run worker with model routing metadata.
	runCtx := llm.WithModelSelection(ctx, spec.LLMRole, spec.LLMLevel, spec.LLMProvider, spec.LLMModel)
	out, err := spec.Run(runCtx, in, env)
	if err != nil {
		return zero, err
	}
	normalizeClientViewUIDs(env, &out)

	// Persist artifact via strategy (only RuntimeState should be cached)
	if err := spec.Strategy.Save(ctx, spec, env, out, fp); err != nil {
		return zero, err
	}

	// If forced, invalidate downstream caches (json-strategy only).
	if env.ForceFrom != "" && env.ForceFrom == strings.ToLower(spec.Key) && env.Resolver != nil {
		for _, d := range spec.Downstream {
			if ds, ok := env.Resolver.Get(d); ok {
				_ = ds.Strategy.Invalidate(ctx, ds, env)
			}
		}
	}
	return out, nil
}

func normalizeClientViewUIDs(env *Env, out *WorkerOutput) {
	if out == nil || out.ClientView == nil {
		return
	}
	view, ok := out.ClientView.(*pipelinev1.ClientView)
	if !ok || view == nil {
		return
	}
	if env != nil && env.UIDGen == nil {
		env.UIDGen = utils.NewUIDGenerator()
	}
	var gen *utils.UIDGenerator
	if env != nil {
		gen = env.UIDGen
	}
	utils.AssignGraphNodeUIDsWithGenerator(gen, view)
}

func ensureArtifact(ctx context.Context, key string, env *Env, visiting map[string]bool) error {
	if env == nil || env.Resolver == nil {
		return fmt.Errorf("runner: resolver is not configured")
	}
	if normalizeKey(key) == "" {
		return fmt.Errorf("runner: empty required worker key")
	}
	spec, ok := env.Resolver.Get(key)
	if !ok {
		fallback := filepath.Join(env.OutDir, normalizeKey(key)+".json")
		if FileExists(env.ArtifactFS, fallback) {
			return nil
		}
		return fmt.Errorf("runner: unknown required worker %s", key)
	}
	path := filepath.Join(env.OutDir, spec.Key+".json")
	if FileExists(env.ArtifactFS, path) {
		return nil
	}
	if visiting == nil {
		visiting = make(map[string]bool)
	}
	specKey := normalizeKey(spec.Key)
	if visiting[specKey] {
		return fmt.Errorf("runner: cyclic worker dependency detected at %s", spec.Key)
	}
	visiting[specKey] = true
	defer delete(visiting, specKey)
	for _, r := range spec.Requires {
		if err := ensureArtifact(ctx, r, env, visiting); err != nil {
			return err
		}
	}
	if err := ExecuteWorker(ctx, spec, env); err != nil {
		return fmt.Errorf("failed to build required worker %s: %w", spec.Key, err)
	}
	return nil
}
