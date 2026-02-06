package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"insightify/internal/llm"
	"insightify/internal/mcp"
	"insightify/internal/runner"
	"insightify/internal/safeio"
	"insightify/internal/utils"
)

// RunContext holds the environment and metadata for a single pipeline execution request.
type RunContext struct {
	ID       string
	RepoName string
	OutDir   string
	Env      *runner.Env
	Cleanup  func()
}

// NewRunContext creates a new context with a unique timestamp-based artifact directory.
func NewRunContext(repoName string) (*RunContext, error) {
	// resolveRepoPaths is defined in main.go (same package)
	name, repoPath, repoFS, err := resolveRepoPaths(repoName)
	if err != nil {
		return nil, err
	}

	// Generate session ID based on timestamp (e.g., 20231027-100000)
	sessionID := time.Now().Format("20060102-150405")

	// Artifacts go to artifacts/<repo>/<session_id>
	outDir := filepath.Join("artifacts", name, sessionID)
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return nil, fmt.Errorf("resolve outDir: %w", err)
	}
	if err := os.MkdirAll(absOutDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir outDir: %w", err)
	}

	artifactFS, err := safeio.NewSafeFS(absOutDir)
	if err != nil {
		return nil, fmt.Errorf("artifact fs: %w", err)
	}

	// Setup LLM (Using FakeClient for Gateway default for now)
	llmCli := llm.Wrap(llm.NewFakeClient(4096), llm.WithHooks())

	env := &runner.Env{
		Repo:       name,
		RepoRoot:   repoPath,
		OutDir:     absOutDir,
		MaxNext:    8,
		RepoFS:     repoFS,
		ArtifactFS: artifactFS,
		ModelSalt:  "gateway|" + sessionID,
		LLM:        llmCli,
		UIDGen:     utils.NewUIDGenerator(),
	}

	// Setup MCP & Registry
	env.MCPHost = mcp.Host{RepoRoot: repoPath, RepoFS: repoFS, ArtifactFS: artifactFS}
	env.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(env.MCP, env.MCPHost)

	// Build Registry
	mainline := runner.BuildRegistryMainline(env)
	codebase := runner.BuildRegistryCodebase(env)
	external := runner.BuildRegistryExternal(env)
	planReg := runner.BuildRegistryPlanDependencies(env)
	env.Resolver = runner.MergeRegistries(mainline, codebase, external, planReg)

	return &RunContext{
		ID:       sessionID,
		RepoName: name,
		OutDir:   absOutDir,
		Env:      env,
		Cleanup:  func() { llmCli.Close() },
	}, nil
}
