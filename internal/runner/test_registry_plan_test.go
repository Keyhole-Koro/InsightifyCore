package runner

import (
	"context"
	"testing"

	"insightify/internal/artifact"
)

func TestPlanRegistryBuildInputInjectsBootstrapWorker(t *testing.T) {
	env := &Env{
		InitCtx: InitContext{Purpose: "Goのランタイムを理解したい"},
		Resolver: MergeRegistries(map[string]WorkerSpec{
			"worker_DAG": {
				Key:         "worker_DAG",
				Description: "Build worker DAG",
			},
			"arch_design": {
				Key:         "arch_design",
				Description: "Architecture planning",
				Requires:    []string{"code_roots"},
			},
		}),
	}

	spec := BuildRegistryPlan(env)["worker_DAG"]
	inAny, err := spec.BuildInput(context.Background(), newDeps(env, spec.Key, nil))
	if err != nil {
		t.Fatalf("BuildInput() error = %v", err)
	}
	in := inAny.(artifact.PlanDependenciesIn)

	workers := map[string]artifact.WorkerMeta{}
	for _, w := range in.Workers {
		workers[w.Key] = w
	}

	planWorker, ok := workers["bootstrap"]
	if !ok {
		t.Fatalf("expected bootstrap worker to be present")
	}
	if planWorker.Description != env.InitCtx.Purpose {
		t.Fatalf("expected bootstrap description %q, got %q", env.InitCtx.Purpose, planWorker.Description)
	}

	workerDAG, ok := workers["worker_DAG"]
	if !ok {
		t.Fatalf("expected worker_DAG to be present")
	}
	if !containsWorkerKey(workerDAG.Requires, "bootstrap") {
		t.Fatalf("expected worker_DAG to require bootstrap, got %v", workerDAG.Requires)
	}
}

func TestPlanRegistryIncludesBootstrapAndCompatibilitySpecs(t *testing.T) {
	env := &Env{}
	reg := BuildRegistryPlan(env)
	if _, ok := reg["bootstrap"]; !ok {
		t.Fatalf("expected bootstrap worker spec in plan registry")
	}
	if _, ok := reg["init_purpose"]; !ok {
		t.Fatalf("expected init_purpose worker spec in plan registry")
	}
}
