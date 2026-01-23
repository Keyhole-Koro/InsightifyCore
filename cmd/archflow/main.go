package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"

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
	provider := flag.String("provider", "gemini", "LLM provider (gemini|groq)")
	model := flag.String("model", "gemini-2.5-pro", "LLM model id (provider-specific)")
	fake := flag.Bool("fake", false, "use a fake LLM (no network)")
	phase := flag.String("phase", "m0", "phase to run (m0|m1|m2|c0|c1|c2|c3|c4|x0|x1)")
	forceFrom := flag.String("force_from", "", "force recompute starting at this phase (e.g., m0|m1|m2|c0|c1|c2)")
	cache := flag.Bool("cache", false, "use cached artifacts (default: off)")
	maxNext := flag.Int("max_next", 8, "max next_files to open/propose")
	tokenCap := flag.Int("token_cap", 0, "max total tokens per scheduler chunk (default depends on provider)")
	flag.Parse()

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

	// Determine requested phase key and default force behavior when cache is disabled
	key := strings.ToLower(strings.TrimSpace(*phase))
	force := strings.ToLower(strings.TrimSpace(*forceFrom))
	if !*cache && force == "" {
		// Default: do not use cache → force recompute of the requested phase
		force = key
	}

	// ----- LLM setup -----
	_ = godotenv.Load()
	// Defer provider-specific API key checks until after flags are parsed
	apiKey := ""
	ctx := context.Background()

	// Prompt hook persists prompts & raw responses under artifacts/prompt/
	ctx = llm.WithPromptHook(ctx, &runner.PromptSaver{Dir: *outDir})

	capacity := *tokenCap
	if capacity <= 0 {
		capacity = defaultTokenCapacity(*provider, *model)
	}

	var base llmclient.LLMClient
	if *fake {
		base = llm.NewFakeClient(capacity)
	} else {
		switch strings.ToLower(strings.TrimSpace(*provider)) {
		case "gemini":
			// Prefer GEMINI_API_KEY for Gemini
			apiKey = os.Getenv("GEMINI_API_KEY")
			if apiKey == "" {
				log.Fatal("GEMINI_API_KEY must be set (or use --fake)")
			}
			base, err = llmclient.NewGeminiClient(ctx, apiKey, *model, capacity)
		case "groq":
			// Prefer GROQ_API_KEY for Groq
			apiKey = os.Getenv("GROQ_API_KEY")
			if apiKey == "" {
				log.Fatal("GROQ_API_KEY must be set (or use --fake)")
			}
			base, err = llmclient.NewGroqClient(apiKey, *model, capacity)
		case "fake":
			base = llm.NewFakeClient(capacity)
			err = nil
		default:
			log.Fatalf("unknown --provider: %s (use gemini|groq|fake)", *provider)
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	cfg := defaultLLMRateConfig(*provider, *model)
	mws := buildRateMiddlewares(cfg)
	mws = append(mws,
		llm.Retry(3, 300*time.Millisecond),
		llm.WithHooks(),
		llm.WithLogging(nil),
	)
	llmCli := llm.Wrap(base, mws...)
	defer llmCli.Close()

	// Scanning is performed per-phase inside runner.BuildRegistryMainline via PlanScan.

	// ----- Build environment & registry -----
	env := &runner.Env{
		Repo:         repoName,
		RepoRoot:     repoPath,
		OutDir:       outAbs,
		MaxNext:      *maxNext,
		RepoFS:       repoFS,
		ArtifactFS:   artifactFS,
		ModelSalt:    os.Getenv("CACHE_SALT") + "|" + *model, // Salt helps invalidate cache when model/prompts change
		ForceFrom:    force,
		LLM:          llmCli,
		Index:        nil,
		MDDocs:       nil,
		StripImgMD:   regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`),
		StripImgHTML: regexp.MustCompile(`(?is)<img[^>]*>`),
	}
	env.MCPHost = mcp.Host{RepoRoot: repoPath, RepoFS: repoFS, ArtifactFS: artifactFS}
	env.MCP = mcp.NewRegistry()
	mcp.RegisterDefaultTools(env.MCP, env.MCPHost)

	mainline := runner.BuildRegistryMainline(env) // m0/m1/m2
	codebase := runner.BuildRegistryCodebase(env) // c0..c4
	external := runner.BuildRegistryExternal(env) // x0..x1
	env.Resolver = runner.MergeRegistries(mainline, codebase, external)

	// ----- Execute requested phase -----
	spec, ok := env.Resolver.Get(key)
	if !ok {
		log.Fatalf("unknown --phase: %s (use m0|m1|m2|c0|c1|c2|c3|c4|x0|x1)", *phase)
	}
	if err := runner.ExecutePhase(ctx, spec, env); err != nil {
		log.Fatal(err)
	}
}

// LLMRateConfig captures rate limiting in a clear, declarative way.
// Zero values disable the corresponding limiter.
type LLMRateConfig struct {
	RPM   int     // requests per minute
	RPD   int     // requests per day
	TPM   int     // tokens per minute (approximate)
	RPS   float64 // legacy requests per second limiter
	Burst int     // burst for RPS
}

// defaultLLMRateConfig defines built-in rate configs per provider/model.
// Example: {RPM: 60} for clarity instead of positional params.
func defaultLLMRateConfig(provider, model string) LLMRateConfig {
	_ = model
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "gemini":
		return LLMRateConfig{RPM: 60, RPS: 1, Burst: 1}
	case "groq":
		return LLMRateConfig{RPM: 30, RPS: 1, Burst: 1}
	default:
		return LLMRateConfig{RPS: 1, Burst: 1}
	}
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

// buildRateMiddlewares converts LLMRateConfig into the corresponding middlewares.
func buildRateMiddlewares(c LLMRateConfig) []llm.Middleware {
	out := []llm.Middleware{}
	if c.RPM > 0 || c.RPD > 0 || c.TPM > 0 {
		out = append(out, llm.MultiLimit(c.RPM, c.RPD, c.TPM))
	}
	if c.RPS > 0 || c.Burst > 0 {
		out = append(out, llm.RateLimit(c.RPS, c.Burst))
	}
	return out
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
