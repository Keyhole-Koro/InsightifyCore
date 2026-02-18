package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"insightify/internal/artifact"
	"insightify/internal/common/safeio"
	llmclient "insightify/internal/llm/client"
	"insightify/internal/mcp"
	"insightify/internal/workers/plan"
)

type testRuntime struct {
	outDir     string
	repoFS     *safeio.SafeFS
	artifactFS *safeio.SafeFS
	resolver   SpecResolver
	mcp        *mcp.Registry
	modelSalt  string
	forceFrom  string
	depsUsage  DepsUsageMode
	llm        llmclient.LLMClient
	artifact   ArtifactStore
}

func (r *testRuntime) GetOutDir() string         { return r.outDir }
func (r *testRuntime) GetRepoFS() *safeio.SafeFS { return r.repoFS }
func (r *testRuntime) Artifacts() ArtifactStore {
	if r.artifact != nil {
		return r.artifact
	}
	return &testArtifactAccess{runtime: r}
}
func (r *testRuntime) GetResolver() SpecResolver   { return r.resolver }
func (r *testRuntime) GetMCP() *mcp.Registry       { return r.mcp }
func (r *testRuntime) GetModelSalt() string        { return r.modelSalt }
func (r *testRuntime) GetForceFrom() string        { return r.forceFrom }
func (r *testRuntime) GetDepsUsage() DepsUsageMode { return r.depsUsage }
func (r *testRuntime) GetLLM() llmclient.LLMClient { return r.llm }

type testArtifactAccess struct {
	runtime *testRuntime
}

func (a *testArtifactAccess) Read(_ context.Context, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(a.runtime.outDir, name))
}

func (a *testArtifactAccess) Write(_ context.Context, name string, content []byte) error {
	path := filepath.Join(a.runtime.outDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func (a *testArtifactAccess) Remove(_ context.Context, name string) error {
	path := filepath.Join(a.runtime.outDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (a *testArtifactAccess) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(a.runtime.outDir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

func TestPlanRegistryBuildInputInjectsBootstrapWorker(t *testing.T) {
	outDir := t.TempDir()
	artifactFS, err := safeio.NewSafeFS(outDir)
	if err != nil {
		t.Fatalf("artifact fs: %v", err)
	}
	seed := plan.BootstrapOut{
		Result: artifact.InitPurposeOut{
			Purpose: "Goのランタイムを理解したい",
		},
		BootstrapContext: artifact.BootstrapContext{
			Purpose: "Goのランタイムを理解したい",
		},
	}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal seed artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "bootstrap.json"), raw, 0o644); err != nil {
		t.Fatalf("write seed artifact: %v", err)
	}

	rt := &testRuntime{
		outDir:     outDir,
		artifactFS: artifactFS,
		resolver: MergeRegistries(map[string]WorkerSpec{
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

	spec := BuildRegistryPlan(rt)["worker_DAG"]
	inAny, err := spec.BuildInput(context.Background(), newDeps(rt, spec.Key, spec.Requires))
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
	if planWorker.Description != "Goのランタイムを理解したい" {
		t.Fatalf("expected bootstrap description %q, got %q", "Goのランタイムを理解したい", planWorker.Description)
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
	rt := &testRuntime{}
	reg := BuildRegistryPlan(rt)
	if _, ok := reg["bootstrap"]; !ok {
		t.Fatalf("expected bootstrap worker spec in plan registry")
	}
	if _, ok := reg["init_purpose"]; !ok {
		t.Fatalf("expected init_purpose worker spec in plan registry")
	}
}
