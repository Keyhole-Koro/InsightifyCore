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
	model := flag.String("model", "gemini-2.5-flash", "Gemini model id")
	phase := flag.String("phase", "p0", "phase to run (p0|p1)")
	force := flag.Bool("force", false, "recompute even if cache exists")
	maxNext := flag.Int("max_next", 8, "max next_files+next_patterns to propose in P0/P1")
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
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY or GOOGLE_API_KEY must be set")
	}

	ctx := context.Background()
	base, err := llm.NewGeminiClient(ctx, apiKey, *model)
	if err != nil {
		log.Fatal(err)
	}
	llmCli := llm.WithHook(base, &PromptSaver{Dir: *outDir})
	defer llmCli.Close()

	// Scan repository (index & markdown docs)
	index, mdDocs, err := scan.IndexRepo(*repo)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("scanned %d files, %d markdown docs", len(index), len(mdDocs))

	switch strings.ToLower(*phase) {
	case "p0":
		runP0(ctx, llmCli, *outDir, index, mdDocs, *maxNext, *force)
	case "p1":
		runP1(ctx, llmCli, *outDir, *repo, *maxNext, *force)
	default:
		log.Fatalf("unknown --phase: %s (use p0|p1)", *phase)
	}
}

func runP0(ctx context.Context, cli llm.LLMClient, outDir string,
	index []t.FileIndexEntry, mdDocs []t.MDDoc, maxNext int, force bool,
) {
	outPath := filepath.Join(outDir, "p0.json")
	if !force && fileExists(outPath) {
		log.Println("P0: using cache →", outPath)
		return
	}
	log.Println("P0: computing…")

	p := pipeline.P0{LLM: cli}
	ctx = llm.WithPhase(ctx, "p0")

	hints := &t.P0Hints{}            // empty by default
	limits := &t.P0Limits{MaxNext: maxNext}

	out, err := p.Run(ctx, index, mdDocs, hints, limits)
	if err != nil {
		log.Fatal(err)
	}
	writeJSON(outDir, "p0.json", out)
	log.Println("P0 →", outPath)
}

func runP1(ctx context.Context, cli llm.LLMClient, outDir, repo string, maxNext int, force bool) {
	inPath := filepath.Join(outDir, "p0.json")
	if !fileExists(inPath) {
		log.Fatalf("P1: missing %s. Run P0 first.", inPath)
	}
	var p0 t.P0Out
	readJSON(outDir, "p0.json", &p0)

	outPath := filepath.Join(outDir, "p1.json")
	if !force && fileExists(outPath) {
		log.Println("P1: using cache →", outPath)
		return
	}
	log.Println("P1: computing…")

	// Prepare opened_files from P0.next_files (top N existing files)
	var opened []t.OpenedFile
	var focus []t.FocusItem
	picked := 0
	for _, nf := range p0.NextFiles {
		if picked >= maxNext {
			break
		}
		full := filepath.Join(repo, filepath.Clean(nf.Path))
		b, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		opened = append(opened, t.OpenedFile{Path: nf.Path, Content: string(b)})
		focus = append(focus, t.FocusItem{Path: nf.Path, WhatToCheck: nf.WhatToCheck})
		picked++
	}

	// Also pass minimal context (index/docs) if helpful
	index, mdDocs, _ := scan.IndexRepo(repo)

	p := pipeline.P1{LLM: cli}
	ctx = llm.WithPhase(ctx, "p1")
	in := t.P1In{
		Previous:     t.P1Previous{ArchitectureHypothesis: p0.ArchitectureHypothesis, NextFiles: p0.NextFiles, NextPatterns: p0.NextPatterns},
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
	writeJSON(outDir, "p1.json", out)
	log.Println("P1 →", outPath)
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
}
