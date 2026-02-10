package runner

import (
	"context"
	"testing"

	"insightify/internal/artifact"
)

func TestPlanRegistryBuildInputInjectsPlanPipelineWorker(t *testing.T) {
	env := &Env{
		InitPurpose: "Goのランタイムを理解したい",
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

	planWorker, ok := workers["plan_pipeline"]
	if !ok {
		t.Fatalf("expected plan_pipeline worker to be present")
	}
	if planWorker.Description != env.InitPurpose {
		t.Fatalf("expected plan_pipeline description %q, got %q", env.InitPurpose, planWorker.Description)
	}

	workerDAG, ok := workers["worker_DAG"]
	if !ok {
		t.Fatalf("expected worker_DAG to be present")
	}
	if !containsWorkerKey(workerDAG.Requires, "plan_pipeline") {
		t.Fatalf("expected worker_DAG to require plan_pipeline, got %v", workerDAG.Requires)
	}
}

func TestPlanRegistryIncludesPlanPipelineSpec(t *testing.T) {
	env := &Env{}
	reg := BuildRegistryPlan(env)
	if _, ok := reg["plan_pipeline"]; !ok {
		t.Fatalf("expected plan_pipeline worker spec in plan registry")
	}
	if _, ok := reg["plan_source_scout"]; !ok {
		t.Fatalf("expected plan_source_scout worker spec in plan registry")
	}
}
