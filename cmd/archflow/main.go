package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"insightify/internal/llm"
	"insightify/internal/pipeline"
	"insightify/internal/scan"
	t "insightify/internal/types"
)

func main() {
	repo := flag.String("repo", "", "path to repository root")
	outDir := flag.String("out", "out", "output directory")
	model := flag.String("model", "gemini-2.5-pro", "Gemini model id")
	mainline := flag.String("mainline", "m0", "mainline to run (m0|m1)")
	force := flag.Bool("force", false, "recompute even if cache exists")
	maxNext := flag.Int("max_next", 8, "max next_files+next_patterns to propose in M0/M1")
	flag.Parse()

	if *repo == "" {
		log.Fatal("--repo is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	_ = godotenv.Load()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY must be set")
	}

	ctx := context.Background()

    // Attach a prompt hook to context so middleware WithHooks() can call it.
    ctx = llm.WithPromptHook(ctx, &PromptSaver{Dir: *outDir})

	// Build a thin Gemini client and compose cross-cutting concerns via middleware.
	base, err := llm.NewGeminiClient(ctx, apiKey, *model)
	if err != nil {
		log.Fatal(err)
	}
	llmCli := llm.Wrap(
		base,
		llm.RateLimitFromEnv("LLM", "GEMINI"), // reads LLM_RPS/LLM_BURST, falls back to GEMINI_*
		llm.Retry(3, 300*time.Millisecond),   // exponential backoff: 300ms, 600ms, 1200ms
		llm.WithHooks(),                      // calls hook.Before/After if present in context
		llm.WithLogging(nil),                 // logs request size & errors; nil => log.Default()
	)
	defer llmCli.Close()

	// Scan repository (index & markdown docs)
	index, mdDocs, err := scan.IndexRepo(*repo)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("scanned %d files, %d markdown docs", len(index), len(mdDocs))

switch strings.ToLower(*mainline) {
case "m0":
    runM0(ctx, llmCli, *outDir, index, mdDocs, *maxNext, *force)
case "m1":
    runM1(ctx, llmCli, *outDir, *repo, *maxNext, *force)
default:
    log.Fatalf("unknown --mainline: %s (use m0|m1)", *mainline)
}
}

func runM0(ctx context.Context, cli llm.LLMClient, outDir string,
    index []t.FileIndexEntry, mdDocs []t.MDDoc, maxNext int, force bool,
) {
    outPath := filepath.Join(outDir, "m0.json")
    if !force && fileExists(outPath) {
        log.Println("M0: using cache →", outPath)
        return
    }
    log.Println("M0: computing…")

    p := pipeline.M0{LLM: cli}
    ctx = llm.WithPhase(ctx, "m0")

    in := t.M0In{
        FileIndex: index,
        MDDocs:    mdDocs,
        Hints:     &t.M0Hints{},
        Limits:    &t.M0Limits{MaxNext: maxNext},
    }
    out, err := p.Run(ctx, in)
    if err != nil {
        log.Fatal(err)
    }
    writeJSON(outDir, "m0.json", out)
    log.Println("M0 →", outPath)
}

func runM1(ctx context.Context, cli llm.LLMClient, outDir, repo string, maxNext int, force bool) {
    inPath := filepath.Join(outDir, "m0.json")
    var m0 t.M0Out
    if fileExists(inPath) {
        readJSON(outDir, "m0.json", &m0)
    } else {
        log.Fatalf("M1: missing m0.json. Run M0 first.")
    }

    outPath := filepath.Join(outDir, "m1.json")
    if !force && fileExists(outPath) {
        log.Println("M1: using cache →", outPath)
        return
    }
    log.Println("M1: computing…")

    // Prepare opened_files from M0.next_files (top N existing files)
    var opened []t.OpenedFile
    var focus []t.FocusQuestion
    picked := 0
    for _, nf := range m0.NextFiles {
        if picked >= maxNext {
            break
        }
        full := filepath.Join(repo, filepath.Clean(nf.Path))
        b, err := os.ReadFile(full)
        if err != nil {
            continue
        }
        opened = append(opened, t.OpenedFile{Path: nf.Path, Content: string(b)})
        // Convert each what_to_check into a FocusQuestion entry
        if len(nf.WhatToCheck) == 0 {
            // If no explicit checks, still create a generic question for this file
            focus = append(focus, t.FocusQuestion{Path: nf.Path, Question: "Review this file for key architecture details"})
        } else {
            for _, q := range nf.WhatToCheck {
                focus = append(focus, t.FocusQuestion{Path: nf.Path, Question: q})
            }
        }
        picked++
    }

    // Also pass minimal context (index/docs) if helpful
    index, mdDocs, _ := scan.IndexRepo(repo)

    p := pipeline.M1{LLM: cli}
    ctx = llm.WithPhase(ctx, "m1")
    in := t.M1In{
        Previous:     m0,
        OpenedFiles:  opened,
        Focus:        focus,
        FileIndex:    index,
        MDDocs:       mdDocs[:min(len(mdDocs), 4)], // keep it light
        LimitMaxNext: maxNext,
    }
    out, err := p.Run(ctx, in)
    if err != nil {
        log.Fatal(err)
    }
    writeJSON(outDir, "m1.json", out)
    log.Println("M1 →", outPath)
}

func writeJSON(dir, name string, v any) {
    b, _ := json.MarshalIndent(v, "", "  ")
    _ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
}

func readJSON(dir, name string, v any) {
    b, err := os.ReadFile(filepath.Join(dir, name))
    if err != nil {
        log.Fatalf("failed to read %s: %v", name, err)
    }
    if err := json.Unmarshal(b, v); err != nil {
        log.Fatalf("failed to unmarshal %s: %v\nraw: %s", name, err, string(b))
    }
}

func fileExists(path string) bool {
    fi, err := os.Stat(path)
    return err == nil && !fi.IsDir()
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

// PromptSaver implements llm.PromptHook to persist prompts & I/O
type PromptSaver struct{ Dir string }

func (p *PromptSaver) Before(ctx context.Context, phase, prompt string, input any) {
    if phase == "" {
        phase = "unknown"
    }
    _ = os.MkdirAll(filepath.Join(p.Dir, "prompt"), 0o755)
    path := filepath.Join(p.Dir, "prompt", phase+".txt")

    var buf bytes.Buffer
    buf.WriteString("==== ")
    buf.WriteString(time.Now().Format(time.RFC3339))
    buf.WriteString(" ====\n")
    buf.WriteString(prompt)
    buf.WriteString("\n\n[INPUT JSON]\n")
    jb, _ := json.MarshalIndent(input, "", "  ")
    buf.Write(jb)
    buf.WriteString("\n\n")

    f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if f != nil {
        _, _ = f.Write(buf.Bytes())
        _ = f.Close()
    }
}

func (p *PromptSaver) After(ctx context.Context, phase string, raw json.RawMessage, err error) {
    if phase == "" {
        phase = "unknown"
    }
    _ = os.MkdirAll(filepath.Join(p.Dir, "prompt"), 0o755)
    path := filepath.Join(p.Dir, "prompt", phase+".txt")

    var buf bytes.Buffer
    buf.WriteString("[RESPONSE]\n")
    if err != nil {
        buf.WriteString("ERROR: " + err.Error() + "\n\n")
    } else {
        buf.Write(raw)
        buf.WriteString("\n\n")
    }
    f, _ := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if f != nil {
        _, _ = f.Write(buf.Bytes())
        _ = f.Close()
    }

    // Also persist latest raw model output for this phase to a handy file
    // like out/m0.raw.json or out/m1.raw.json for quick debugging when JSON
    // parsing/normalization fails later in the pipeline.
    _ = os.WriteFile(filepath.Join(p.Dir, phase+".raw.json"), raw, 0o644)
}
