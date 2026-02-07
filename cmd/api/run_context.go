package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"insightify/internal/globalctx"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/mcp"
	"insightify/internal/runner"
	"insightify/internal/safeio"
	"insightify/internal/scan"
	"insightify/internal/utils"
)

func buildGatewayLLM() (*llm.InMemoryModelRegistry, error) {
	reg := llm.NewInMemoryModelRegistry()
	if err := llm.RegisterFakeModels(reg); err != nil {
		return nil, err
	}
	roles := []llm.ModelRole{llm.ModelRoleWorker, llm.ModelRolePlanner}
	levels := []llm.ModelLevel{llm.ModelLevelLow, llm.ModelLevelMiddle, llm.ModelLevelHigh, llm.ModelLevelXHigh}
	for _, role := range roles {
		for _, level := range levels {
			if err := reg.SetDefault(role, level, "fake", llm.FakeModelByLevel(level)); err != nil {
				return nil, err
			}
		}
	}
	return reg, nil
}

// RunContext holds the environment and metadata for a single pipeline execution request.
type RunContext struct {
	ID       string
	RepoName string
	OutDir   string
	Env      *runner.Env
	Cleanup  func()
}

// NewRunContext creates a new context bound to the provided session ID.
// If sessionID is empty, it falls back to a timestamp.
func NewRunContext(repoName string, sessionID string) (*RunContext, error) {
	name, repoPath, repoFS, err := resolveRepoPaths(repoName)
	if err != nil {
		return nil, err
	}

	if sessionID == "" {
		sessionID = time.Now().Format("20060102-150405")
	}
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

	reg, err := buildGatewayLLM()
	if err != nil {
		return nil, err
	}

	baseCtx := globalctx.WithGlobalContext(context.Background(), globalctx.GlobalContext{
		ModelSelectionMode: globalctx.ModelSelectionModePreferAvailable,
		ProviderTiers: map[string]string{
			"gemini": "free",
			"groq":   "free",
		},
	})

	fallback, err := reg.BuildClient(baseCtx, llm.ModelRoleWorker, llm.ModelLevelMiddle, "", "", 4096)
	if err != nil {
		return nil, err
	}

	llmCli := llm.Wrap(
		llm.NewModelDispatchClient(fallback),
		llm.SelectModel(reg, 4096),
		llm.RespectRateLimitSignals(llmclient.HeaderRateLimitControlAdapter{}),
		llm.WithUsageLedger(filepath.Join(filepath.Dir(absOutDir), "llm_usage_daily.json")),
		llm.WithHooks(),
	)

	env := &runner.Env{
		Repo:       name,
		RepoRoot:   repoPath,
		OutDir:     absOutDir,
		MaxNext:    8,
		RepoFS:     repoFS,
		ArtifactFS: artifactFS,
		ModelSalt:  "gateway|" + reg.DefaultsSalt(),
		LLM:        llmCli,
		UIDGen:     utils.NewUIDGenerator(),
	}

	env.MCPHost = mcp.Host{RepoRoot: repoPath, ReposRoot: scan.ReposDir(), RepoFS: repoFS, ArtifactFS: artifactFS}
	env.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(env.MCP, env.MCPHost)

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
