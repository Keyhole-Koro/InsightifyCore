package main

import (
	"context"
	"flag"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"insightify/internal/llm"

	"insightify/internal/runner"
)

func main() {
	// ----- Flags -----
	repo := flag.String("repo", "", "path to repository root")
	outDir := flag.String("out", "artifacts", "output directory")
    provider := flag.String("provider", "gemini", "LLM provider (gemini|groq)")
    model := flag.String("model", "gemini-2.5-pro", "LLM model id (provider-specific)")
	fake := flag.Bool("fake", false, "use a fake LLM (no network)")
    phase := flag.String("phase", "m0", "phase to run (m0|m1|m2|x0|x1|x2)")
    forceFrom := flag.String("force_from", "", "force recompute starting at this phase (e.g., m0|m1|m2|x0|x1|x2)")
    cache := flag.Bool("cache", false, "use cached artifacts (default: off)")
	maxNext := flag.Int("max_next", 8, "max next_files to open/propose")
	flag.Parse()

	if *repo == "" {
		log.Fatal("--repo is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

    // Determine requested phase key and default force behavior when cache is disabled
    key := strings.ToLower(strings.TrimSpace(*phase))
    force := strings.ToLower(strings.TrimSpace(*forceFrom))
    if !*cache && force == "" {
        // Default: do not use cache → force recompute of the requested phase
        force = key
    }

    // ----- LLM setup -----
	_ = godotenv.Load()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" && !*fake {
		log.Fatal("GEMINI_API_KEY must be set (or use --fake)")
	}
	ctx := context.Background()

	// Prompt hook persists prompts & raw responses under artifacts/prompt/
	ctx = llm.WithPromptHook(ctx, &runner.PromptSaver{Dir: *outDir})

    var base llm.LLMClient
    var err error
    if *fake {
        base = llm.NewFakeClient()
    } else {
        switch strings.ToLower(strings.TrimSpace(*provider)) {
        case "gemini":
            base, err = llm.NewGeminiClient(ctx, apiKey, *model)
        case "groq":
            base, err = llm.NewGroqClient(apiKey, *model)
        default:
            log.Fatalf("unknown --provider: %s (use gemini|groq)", *provider)
        }
        if err != nil { log.Fatal(err) }
    }
	llmCli := llm.Wrap(
		base,
		llm.RateLimitFromEnv("LLM", "GEMINI"),
		llm.Retry(3, 300*time.Millisecond),
		llm.WithHooks(),
		llm.WithLogging(nil),
	)
	defer llmCli.Close()

	// Scanning is performed per-phase inside runner.BuildRegistry via PlanScan.

	// ----- Build environment & registry -----
    env := &runner.Env{
        Repo:         *repo,
        OutDir:       *outDir,
        MaxNext:      *maxNext,
        ModelSalt:    os.Getenv("CACHE_SALT") + "|" + *model, // Salt helps invalidate cache when model/prompts change
        ForceFrom:    force,
        LLM:          llmCli,
        Index:        nil,
        MDDocs:       nil,
        StripImgMD:   regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`),
        StripImgHTML: regexp.MustCompile(`(?is)<img[^>]*>`),
    }

	reg := runner.BuildRegistry(env)       // m0/m1/m2
	for k, v := range runner.BuildX(env) { // x0/x1
		reg[k] = v
	}

	// ----- Execute requested phase -----
    spec, ok := reg[key]
	if !ok {
		log.Fatalf("unknown --phase: %s (use m0|m1|m2|x0|x1|x2)", *phase)
	}
	if err := runner.ExecutePhase(ctx, spec, env, reg); err != nil {
		log.Fatal(err)
	}
}
