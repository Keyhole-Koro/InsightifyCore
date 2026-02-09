package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"insightify/internal/llm"
	"insightify/internal/mcp"
	"insightify/internal/safeio"
	"insightify/internal/scan"
)

func TestAllWorkersExecuteWithFakeLLM_ArchitectureRegistry(t *testing.T) {
	env := newWorkerIntegrationEnv(t)
	resolver := MergeRegistries(
		BuildRegistryCodebase(env),
		BuildRegistryArchitecture(env),
		BuildRegistryExternal(env),
		BuildRegistryPlan(env),
	)
	env.Resolver = resolver

	runAllWorkersAndAssertArtifacts(t, env)
}

func TestAllWorkersExecuteWithFakeLLM_MainlineRegistry(t *testing.T) {
	env := newWorkerIntegrationEnv(t)
	resolver := MergeRegistries(
		BuildRegistryCodebase(env),
		BuildRegistryArchitecture(env),
		BuildRegistryExternal(env),
		BuildRegistryPlan(env),
	)
	env.Resolver = resolver

	runAllWorkersAndAssertArtifacts(t, env)
}

func runAllWorkersAndAssertArtifacts(t *testing.T, env *Env) {
	t.Helper()
	ctx := context.Background()
	for _, spec := range env.Resolver.List() {
		if err := ExecuteWorker(ctx, spec, env); err != nil {
			t.Fatalf("execute worker %s: %v", spec.Key, err)
		}
		path := filepath.Join(env.OutDir, spec.Key+".json")
		if !FileExists(env.ArtifactFS, path) {
			t.Fatalf("artifact not found for worker %s: %s", spec.Key, path)
		}
	}
}

func newWorkerIntegrationEnv(t *testing.T) *Env {
	t.Helper()

	base := t.TempDir()
	reposDir := filepath.Join(base, "repos")
	repoName := "sample-repo"
	repoRoot := filepath.Join(reposDir, repoName)
	outDir := filepath.Join(base, "artifacts")

	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	writeTestRepoFixture(t, repoRoot)

	repoFS, err := safeio.NewSafeFS(repoRoot)
	if err != nil {
		t.Fatalf("repo fs: %v", err)
	}
	artifactFS, err := safeio.NewSafeFS(outDir)
	if err != nil {
		t.Fatalf("artifact fs: %v", err)
	}

	prevReposDir := scan.ReposDir()
	prevScanFS := scan.CurrentSafeFS()
	prevDefaultFS := safeio.Default()
	scan.SetReposDir(reposDir)
	scan.SetSafeFS(repoFS)
	safeio.SetDefault(repoFS)
	t.Cleanup(func() {
		safeio.SetDefault(prevDefaultFS)
		scan.SetSafeFS(prevScanFS)
		scan.SetReposDir(prevReposDir)
		scan.ClearCache()
	})

	reg := mcp.NewRegistry()
	host := mcp.Host{
		RepoRoot:   repoRoot,
		RepoFS:     repoFS,
		ArtifactFS: artifactFS,
	}
	mcp.RegisterDefaultTools(reg, host)

	return &Env{
		Repo:       repoName,
		RepoRoot:   repoRoot,
		OutDir:     outDir,
		MaxNext:    1,
		RepoFS:     repoFS,
		ArtifactFS: artifactFS,
		MCP:        reg,
		MCPHost:    host,
		ModelSalt:  "test-salt",
		DepsUsage:  DepsUsageError,
		LLM:        llm.NewFakeClient(4096),
	}
}

func writeTestRepoFixture(t *testing.T, repoRoot string) {
	t.Helper()
	files := map[string]string{
		"README.md": "sample repo for worker integration tests\n",
		"go.mod":    "module example.com/sample\n\ngo 1.22\n",
		"cmd/app/main.go": `package main

import "example.com/sample/internal/core"

func main() {
	_ = core.Run()
}
`,
		"internal/core/core.go": `package core

func Run() string {
	return "ok"
}
`,
		".github/workflows/ci.yml": "name: ci\n",
		"config/app.yaml":          "service: sample\n",
	}
	for rel, content := range files {
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}
