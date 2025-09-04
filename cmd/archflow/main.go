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
	model := flag.String("model", "gemini-2.5-pro", "Gemini model id")
	fake := flag.Bool("fake", false, "use a fake LLM (no network)")
    phase := flag.String("phase", "m0", "phase to run (m0|m1|m2|x0|x1|x2)")
    forceFrom := flag.String("force_from", "", "force recompute starting at this phase (e.g., m0|m1|m2|x0|x1|x2)")
	maxNext := flag.Int("max_next", 8, "max next_files to open/propose")
	flag.Parse()

	if *repo == "" {
		log.Fatal("--repo is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
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
		base, err = llm.NewGeminiClient(ctx, apiKey, *model)
		if err != nil {
			log.Fatal(err)
		}
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
		ForceFrom:    strings.ToLower(strings.TrimSpace(*forceFrom)),
		LLM:          llmCli,
		Index:        nil,
		MDDocs:       nil,
		ExtCounts:    map[string]int{},
		StripImgMD:   regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`),
		StripImgHTML: regexp.MustCompile(`(?is)<img[^>]*>`),
	}

	reg := runner.BuildRegistry(env)       // m0/m1/m2
	for k, v := range runner.BuildX(env) { // x0/x1
		reg[k] = v
	}

	// ----- Execute requested phase -----
	key := strings.ToLower(strings.TrimSpace(*phase))
	spec, ok := reg[key]
    if !ok {
        log.Fatalf("unknown --phase: %s (use m0|m1|m2|x0|x1|x2)", *phase)
    }
	if err := runner.ExecutePhase(ctx, spec, env, reg); err != nil {
		log.Fatal(err)
	}
}
