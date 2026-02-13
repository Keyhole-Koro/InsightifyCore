package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// RunEnvironment abstracts the worker execution environment.
type RunEnvironment interface {
	GetEnv() *runner.Env
	GetOutDir() string
	GetID() string
}

// RunContext holds the full runtime environment for a single project run.
type RunContext struct {
	ID       string
	RepoName string
	OutDir   string
	Env      *runner.Env
	Cleanup  func()
}

// RunEnvironment interface implementation.
func (r *RunContext) GetEnv() *runner.Env { return r.Env }
func (r *RunContext) GetOutDir() string   { return r.OutDir }
func (r *RunContext) GetID() string       { return r.ID }

type resolvedSources struct {
	Name        string
	SourcePaths []string
	PrimaryPath string
	PrimaryFS   *safeio.SafeFS
}

func resolveRunSources(repoName string) (resolvedSources, error) {
	trimmedRepo := strings.TrimSpace(repoName)
	if trimmedRepo == "" {
		root := scan.ReposDir()
		if strings.TrimSpace(root) == "" {
			root = "."
		}
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		fs, err := safeio.NewSafeFS(root)
		if err != nil {
			return resolvedSources{}, fmt.Errorf("bootstrap repo fs: %w", err)
		}
		return resolvedSources{Name: "bootstrap", SourcePaths: []string{root}, PrimaryPath: root, PrimaryFS: fs}, nil
	}

	name, repoPath, repoFS, err := resolveRepoPaths(trimmedRepo)
	if err != nil {
		return resolvedSources{}, err
	}
	return resolvedSources{Name: name, SourcePaths: []string{repoPath}, PrimaryPath: repoPath, PrimaryFS: repoFS}, nil
}

func buildGatewayLLM() (*llm.InMemoryModelRegistry, error) {
	reg := llm.NewInMemoryModelRegistry()
	if err := llm.RegisterFakeModels(reg); err != nil {
		return nil, err
	}

	defaultProvider := "fake"
	defaultsByLevel := map[llm.ModelLevel]string{
		llm.ModelLevelLow:    llm.FakeModelByLevel(llm.ModelLevelLow),
		llm.ModelLevelMiddle: llm.FakeModelByLevel(llm.ModelLevelMiddle),
		llm.ModelLevelHigh:   llm.FakeModelByLevel(llm.ModelLevelHigh),
		llm.ModelLevelXHigh:  llm.FakeModelByLevel(llm.ModelLevelXHigh),
	}

	if strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != "" {
		tier := strings.TrimSpace(os.Getenv("GEMINI_TIER"))
		if tier == "" {
			tier = "free"
		}
		if err := llmclient.RegisterGeminiModelsForTier(reg, tier); err != nil {
			return nil, err
		}
		defaultProvider = "gemini"
		defaultsByLevel = map[llm.ModelLevel]string{
			llm.ModelLevelLow:    "gemini-2.5-flash",
			llm.ModelLevelMiddle: "gemini-2.5-flash",
			llm.ModelLevelHigh:   "gemini-2.5-pro",
			llm.ModelLevelXHigh:  "gemini-2.5-pro",
		}
	} else if strings.TrimSpace(os.Getenv("GROQ_API_KEY")) != "" {
		tier := strings.TrimSpace(os.Getenv("GROQ_TIER"))
		if tier == "" {
			tier = "free"
		}
		if err := llmclient.RegisterGroqModelsForTier(reg, tier); err != nil {
			return nil, err
		}
		defaultProvider = "groq"
		defaultsByLevel = map[llm.ModelLevel]string{
			llm.ModelLevelLow:    "llama-3.1-8b-instant",
			llm.ModelLevelMiddle: "llama-3.3-70b-versatile",
			llm.ModelLevelHigh:   "llama-3.3-70b-versatile",
			llm.ModelLevelXHigh:  "openai/gpt-oss-120b",
		}
	}

	roles := []llm.ModelRole{llm.ModelRoleWorker, llm.ModelRolePlanner}
	levels := []llm.ModelLevel{llm.ModelLevelLow, llm.ModelLevelMiddle, llm.ModelLevelHigh, llm.ModelLevelXHigh}
	for _, role := range roles {
		for _, level := range levels {
			model := strings.TrimSpace(defaultsByLevel[level])
			if model == "" {
				model = llm.FakeModelByLevel(level)
			}
			if err := reg.SetDefault(role, level, defaultProvider, model); err != nil {
				return nil, err
			}
		}
	}
	return reg, nil
}

// NewRunContext constructs the full runtime environment for a project run.
func NewRunContext(repoName string, projectID string) (*RunContext, error) {
	sources, err := resolveRunSources(repoName)
	if err != nil {
		return nil, err
	}

	if projectID == "" {
		projectID = time.Now().Format("20060102-150405")
	}
	outDir := filepath.Join("artifacts", sources.Name, projectID)
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
		ProviderTiers:      map[string]string{"gemini": "free", "groq": "free"},
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
		Repo:        sources.Name,
		RepoRoot:    sources.PrimaryPath,
		SourcePaths: append([]string(nil), sources.SourcePaths...),
		OutDir:      absOutDir,
		MaxNext:     8,
		RepoFS:      sources.PrimaryFS,
		ArtifactFS:  artifactFS,
		ModelSalt:   "gateway|" + reg.DefaultsSalt(),
		LLM:         llmCli,
		UIDGen:      utils.NewUIDGenerator(),
	}

	env.MCPHost = mcp.Host{RepoRoot: sources.PrimaryPath, ReposRoot: scan.ReposDir(), RepoFS: sources.PrimaryFS, ArtifactFS: artifactFS}
	env.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(env.MCP, env.MCPHost)

	architecture := runner.BuildRegistryArchitecture(env)
	codebase := runner.BuildRegistryCodebase(env)
	external := runner.BuildRegistryExternal(env)
	planReg := runner.BuildRegistryPlan(env)
	testReg := runner.BuildRegistryTest(env)
	env.Resolver = runner.MergeRegistries(architecture, codebase, external, planReg, testReg)

	return &RunContext{ID: projectID, RepoName: sources.Name, OutDir: absOutDir, Env: env, Cleanup: func() { llmCli.Close() }}, nil
}

// resolveRepoPaths resolves repository name to filesystem paths.
func resolveRepoPaths(repo string) (string, string, *safeio.SafeFS, error) {
	repoName := strings.TrimSpace(repo)
	if repoName == "" {
		return "", "", nil, fmt.Errorf("--repo is required")
	}
	reposRoot := strings.TrimSpace(os.Getenv("REPOS_ROOT"))
	if reposRoot == "" {
		return "", "", nil, fmt.Errorf("REPOS_ROOT must be set")
	}
	if abs, err := filepath.Abs(reposRoot); err == nil {
		reposRoot = abs
	}
	scan.SetReposDir(reposRoot)
	repoPath, err := scan.ResolveRepo(repoName)
	if err != nil {
		return "", "", nil, err
	}
	sfs, err := safeio.NewSafeFS(repoPath)
	if err != nil {
		return "", "", nil, err
	}
	return repoName, repoPath, sfs, nil
}
