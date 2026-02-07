package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"insightify/internal/globalctx"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
	"insightify/internal/safeio"

	"insightify/internal/mcp"
	"insightify/internal/runner"
	"insightify/internal/scan"
)

func main() {
	// ----- Flags -----
	repo := flag.String("repo", "", "repository folder name under ./repos")
	outDir := flag.String("out", "artifacts", "output directory")
	provider := flag.String("provider", "gemini", "LLM provider (gemini|groq|fake)")
	model := flag.String("model", "gemini-2.5-pro", "LLM model id (provider-specific)")
	fake := flag.Bool("fake", false, "use fake LLM defaults for all levels")
	phase := flag.String("phase", "code_roots", "phase to run (code_roots|code_specs|code_imports|code_graph|code_tasks|code_symbols|arch_design|infra_context|infra_refine)")
	forceFrom := flag.String("force_from", "", "force recompute starting at this phase (e.g., c0|c1|m1)")
	cache := flag.Bool("cache", false, "use cached artifacts (default: off)")
	maxNext := flag.Int("max_next", 8, "max follow-up evidence requests")
	tokenCap := flag.Int("token_cap", 0, "max total tokens per scheduler chunk (default depends on provider)")
	viz := flag.Bool("viz", false, "print phase dependency graph (mermaid) and exit")
	flag.Parse()

	if *viz {
		fmt.Println(runner.GenerateMermaidGraph())
		return
	}

	_ = godotenv.Load()

	repoName, repoPath, repoFS, err := resolveRepoPaths(*repo)
	if err != nil {
		log.Fatal(err)
	}
	outAbs, err := filepath.Abs(*outDir)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(outAbs, 0o755); err != nil {
		log.Fatal(err)
	}
	artifactFS, err := safeio.NewSafeFS(outAbs)
	if err != nil {
		log.Fatal(err)
	}
	safeio.SetDefault(repoFS)

	key := strings.ToLower(strings.TrimSpace(*phase))
	force := strings.ToLower(strings.TrimSpace(*forceFrom))
	if !*cache && force == "" {
		force = key
	}

	ctx := globalctx.WithGlobalContext(context.Background(), globalctx.GlobalContext{
		ModelSelectionMode: globalctx.ModelSelectionModePreferAvailable,
		ProviderTiers: map[string]string{
			"gemini": "free",
			"groq":   "free",
		},
	})
	ctx = llm.WithPromptHook(ctx, &llm.PromptSaver{Dir: *outDir})

	capacity := *tokenCap
	if capacity <= 0 {
		capacity = defaultTokenCapacity(*provider, *model)
	}

	reg, err := setupModelRegistry(ctx, *provider, *model, *fake)
	if err != nil {
		log.Fatal(err)
	}

	fallback, err := reg.BuildClient(ctx, llm.ModelRoleWorker, llm.ModelLevelMiddle, "", "", capacity)
	if err != nil {
		log.Fatal(err)
	}

	mws := []llm.Middleware{
		llm.SelectModel(reg, capacity),
		llm.RespectRateLimitSignals(llmclient.HeaderRateLimitControlAdapter{}),
		llm.WithUsageLedger(filepath.Join(outAbs, "llm_usage_daily.json")),
	}
	mws = append(mws,
		llm.Retry(3, 300*time.Millisecond),
		llm.WithHooks(),
		llm.WithLogging(nil),
	)

	dispatch := llm.NewModelDispatchClient(fallback)
	llmCli := llm.Wrap(dispatch, mws...)
	defer llmCli.Close()

	env := &runner.Env{
		Repo:       repoName,
		RepoRoot:   repoPath,
		OutDir:     outAbs,
		MaxNext:    *maxNext,
		RepoFS:     repoFS,
		ArtifactFS: artifactFS,
		ModelSalt:  os.Getenv("CACHE_SALT") + "|" + reg.DefaultsSalt(),
		ForceFrom:  force,
		LLM:        llmCli,
		Index:      nil,
		MDDocs:     nil,
	}
	env.MCPHost = mcp.Host{RepoRoot: repoPath, ReposRoot: scan.ReposDir(), RepoFS: repoFS, ArtifactFS: artifactFS}
	env.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(env.MCP, env.MCPHost)

	architecture := runner.BuildRegistryArchitecture(env)
	codebase := runner.BuildRegistryCodebase(env)
	external := runner.BuildRegistryExternal(env)
	planReg := runner.BuildRegistryPlanDependencies(env)
	env.Resolver = runner.MergeRegistries(architecture, codebase, external, planReg)

	spec, ok := env.Resolver.Get(key)
	if !ok {
		log.Fatalf("unknown --phase: %s (use code_roots|code_specs|code_imports|code_graph|code_tasks|code_symbols|arch_design|infra_context|infra_refine)", *phase)
	}
	if err := runner.ExecuteWorker(ctx, spec, env); err != nil {
		log.Fatal(err)
	}
}

type modelBinding struct {
	provider string
	model    string
}

func setupModelRegistry(ctx context.Context, defaultProvider, defaultModel string, forceFake bool) (*llm.InMemoryModelRegistry, error) {
	reg := llm.NewInMemoryModelRegistry()
	geminiTier := globalctx.ProviderTierFrom(ctx, "gemini", "free")
	groqTier := globalctx.ProviderTierFrom(ctx, "groq", "free")
	if err := llmclient.RegisterGeminiModelsForTier(reg, geminiTier); err != nil {
		return nil, err
	}
	if err := llmclient.RegisterGroqModelsForTier(reg, groqTier); err != nil {
		return nil, err
	}
	if err := llm.RegisterFakeModels(reg); err != nil {
		return nil, err
	}

	roles := []llm.ModelRole{llm.ModelRoleWorker, llm.ModelRolePlanner}
	levels := []llm.ModelLevel{llm.ModelLevelLow, llm.ModelLevelMiddle, llm.ModelLevelHigh, llm.ModelLevelXHigh}
	for _, role := range roles {
		for _, level := range levels {
			binding := resolveModelBinding(role, level, defaultProvider, defaultModel)
			if forceFake {
				binding = modelBinding{provider: "fake", model: llm.FakeModelByLevel(level)}
			}
			if err := reg.SetDefault(role, level, binding.provider, binding.model); err != nil {
				return nil, err
			}
		}
	}

	return reg, nil
}

func resolveModelBinding(role llm.ModelRole, level llm.ModelLevel, defaultProvider, defaultModel string) modelBinding {
	provider := firstNonEmpty(
		os.Getenv("LLM_"+strings.ToUpper(string(role))+"_"+strings.ToUpper(string(level))+"_PROVIDER"),
		os.Getenv("LLM_"+strings.ToUpper(string(role))+"_PROVIDER"),
		os.Getenv("LLM_PROVIDER"),
		defaultProvider,
	)
	model := firstNonEmpty(
		os.Getenv("LLM_"+strings.ToUpper(string(role))+"_"+strings.ToUpper(string(level))+"_MODEL"),
		os.Getenv("LLM_"+strings.ToUpper(string(role))+"_MODEL"),
		os.Getenv("LLM_MODEL"),
		defaultModel,
	)
	// Env/flag values often include accidental whitespace; provider is
	// compared as a key, so normalize case as well.
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if model == "" {
		model = fallbackModelForProvider(provider, level)
	}
	return modelBinding{provider: provider, model: model}
}

func fallbackModelForProvider(provider string, level llm.ModelLevel) string {
	switch provider {
	case "gemini":
		if level == llm.ModelLevelHigh || level == llm.ModelLevelXHigh {
			return "gemini-2.5-pro"
		}
		return "gemini-2.5-flash"
	case "groq":
		if level == llm.ModelLevelLow {
			return "llama-3.1-8b-instant"
		}
		return "llama-3.3-70b-versatile"
	case "fake":
		return llm.FakeModelByLevel(level)
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		// Treat whitespace-only values as unset.
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func defaultTokenCapacity(provider, model string) int {
	_ = model
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gemini":
		return 12000
	case "groq":
		return 6000
	default:
		return 1000
	}
}

func resolveRepoPaths(repo string) (string, string, *safeio.SafeFS, error) {
	repoName := strings.TrimSpace(repo)
	if repoName == "" {
		return "", "", nil, fmt.Errorf("--repo is required")
	}
	reposRoot := strings.TrimSpace(os.Getenv("REPOS_ROOT"))
	if reposRoot == "" {
		return "", "", nil, fmt.Errorf("REPOS_ROOT must be set to the absolute repositories directory")
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
		return "", "", nil, fmt.Errorf("failed to initialize safe filesystem: %w", err)
	}
	return repoName, repoPath, sfs, nil
}
