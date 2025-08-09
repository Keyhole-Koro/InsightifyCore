package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/joho/godotenv"

	"insightify/internal/llm"
	"insightify/internal/pipeline"
	"insightify/internal/scan"
	t "insightify/internal/types"
	"insightify/internal/util/jsonutil"
)

func main() {
	repo := flag.String("repo", "", "path to the repository root")
	model := flag.String("model", "gemini-2.5-flash", "Gemini model id")
	outDir := flag.String("out", "out", "output directory")
	phase := flag.String("phase", "", "start phase whose results and later phases should be taken from cache unless --force (p0|p1|p2|p3|p4)")
	force := flag.Bool("force", false, "recompute specified phase and later phases, ignoring cache")

	// concurrency controls for per-dir LLM calls (P2 / P4 map)
	concurrency := flag.Int("concurrency", 4, "max parallel LLM calls for per-dir phases (P2/P4 map)")
	qps := flag.Float64("qps", 2.0, "approx max LLM requests per second (token-bucket-ish)")

	flag.Parse()
	if *repo == "" {
		log.Fatal("--repo is required")
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	// Load env; accept GEMINI_API_KEY or GOOGLE_API_KEY
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

	// Simple concurrency limiter + naive rate limiter (shared across goroutines)
	sem := make(chan struct{}, *concurrency)
	var tickC <-chan time.Time
	var ticker *time.Ticker
	if *qps > 0 {
		d := time.Duration(float64(time.Second) / *qps)
		if d < time.Millisecond {
			d = time.Millisecond
		}
		ticker = time.NewTicker(d)
		tickC = ticker.C
		defer ticker.Stop()
	}
	acquire := func() {
		sem <- struct{}{}
		if tickC != nil {
			<-tickC // wait one token
		}
	}
	release := func() { <-sem }

	// Scan repo (always fresh; cheap & deterministic)
	tree, err := scan.Scan(*repo)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("scanned %d files in %s", len(tree.Files), *repo)

	// Light context for P0
	heads := scan.ExtractDocHeadings(*repo, tree)
	manifests := scan.CollectManifests(*repo, tree)

	// Cache policy
	phaseIdx := phaseIndex(*phase)
	useCacheFor := func(i int) bool { return phaseIdx >= 0 && i >= phaseIdx && !(*force) }

	// ------------------------- P0 -------------------------------------------
	var p0Out t.P0Out
	if useCacheFor(0) && fileExists(filepath.Join(*outDir, "p0.json")) {
		log.Println("P0: using cache")
		readJSON(*outDir, "p0.json", &p0Out)
	} else {
		log.Println("P0: computing…")
		p0 := pipeline.P0{LLM: llmCli}
		// Compact tree summary for P0 (path/ext/loc only)
		treeSummary := make([]map[string]any, 0, len(tree.Files))
		for _, f := range tree.Files {
			treeSummary = append(treeSummary, map[string]any{"path": f.Path, "ext": f.Ext, "loc": f.LOC})
		}
		ctxP0 := llm.WithPhase(ctx, "p0")
		p0Out, err = p0.Run(ctxP0, treeSummary, heads, manifests, nil)
		if err != nil {
			log.Fatal(err)
		}
		writeJSON(*outDir, "p0.json", p0Out)
	}

	// ------------------------- P1 -------------------------------------------
	var p1Out t.P1Out
	if useCacheFor(1) && fileExists(filepath.Join(*outDir, "p1.json")) {
		log.Println("P1: using cache")
		readJSON(*outDir, "p1.json", &p1Out)
	} else {
		log.Println("P1: computing…")
		var docSnips, entrySnips []any
		for _, d := range p0Out.TopDocs {
			if fm, ok := tree.FindByPath(d.Path); ok {
				docSnips = append(docSnips, scan.Extract(tree, fm))
			}
		}
		for _, e := range p0Out.EntryPoints {
			if fm, ok := tree.FindByPath(e.Path); ok {
				entrySnips = append(entrySnips, scan.Extract(tree, fm))
			}
		}
		p1 := pipeline.P1{LLM: llmCli}
		ctxP1 := llm.WithPhase(ctx, "p1")
		var err error
		p1Out, err = p1.Run(ctxP1, docSnips, entrySnips, manifests)
		if err != nil {
			log.Fatal(err)
		}
		writeJSON(*outDir, "p1.json", p1Out)
	}

	// ------------------------- P2 (parallel) --------------------------------
	var allP2 []t.P2Out
	p2 := pipeline.P2{LLM: llmCli}

	// Build dir set from read targets (as a set)
	dirSet := make(map[string]struct{})
	for _, rt := range p1Out.ReadTargets {
		dirSet[filepath.Dir(rt.Path)] = struct{}{}
	}

	{
		var mu sync.Mutex
		g, gctx := errgroup.WithContext(ctx)

		for dir := range dirSet {
			dir := dir // capture
			g.Go(func() error {
				f := fmt.Sprintf("p2_%s.json", safe(dir))
				if useCacheFor(2) && fileExists(filepath.Join(*outDir, f)) {
					log.Printf("P2: using cache for %s", dir)
					var out t.P2Out
					readJSON(*outDir, f, &out)
					mu.Lock()
					allP2 = append(allP2, out)
					mu.Unlock()
					return nil
				}

				// prepare snippets (up to 3)
				var fmList []scan.FileMeta
				for _, fm := range tree.Files {
					if filepath.Dir(fm.Path) == dir {
						fmList = append(fmList, fm)
					}
				}
				var snips []scan.Snippet
				for i, fm := range fmList {
					if i >= 3 {
						break
					}
					snips = append(snips, scan.Extract(tree, fm))
				}

				acquire()
				defer release()
				ctxP2 := llm.WithPhase(gctx, "p2:"+safe(dir))
				out, err := p2.Run(ctxP2, dir, snips, p1Out.Taxonomy, p1Out.Glossary)
				if err != nil {
					log.Printf("P2 error (%s): %v", dir, err)
					return nil // keep going
				}
				writeJSON(*outDir, f, out)
				mu.Lock()
				allP2 = append(allP2, out)
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			log.Printf("P2 parallel: %v", err)
		}
	}

	// ------------------------- P3 -------------------------------------------
	var p3Out t.P3Out
	if useCacheFor(3) && fileExists(filepath.Join(*outDir, "p3.json")) {
		log.Println("P3: using cache")
		readJSON(*outDir, "p3.json", &p3Out)
	} else {
		log.Println("P3: computing…")
		p3 := pipeline.P3{LLM: llmCli}
		ctxP3 := llm.WithPhase(ctx, "p3")
		var err error
		p3Out, err = p3.Run(ctxP3, allP2, p1Out.Glossary, p1Out.Taxonomy)
		if err != nil {
			log.Fatal(err)
		}
		writeJSON(*outDir, "p3.json", p3Out)
	}

	// ------------------------- P4 Map (parallel) -----------------------------
	var mapBatches []t.P4Out

	// if dirSet is empty, rebuild from P1 read targets
	if len(dirSet) == 0 {
		dirSet = map[string]struct{}{}
		for _, rt := range p1Out.ReadTargets {
			dirSet[filepath.Dir(rt.Path)] = struct{}{}
		}
	}

	{
		var mu sync.Mutex
		g, gctx := errgroup.WithContext(ctx)

		for dir := range dirSet {
			dir := dir // capture
			g.Go(func() error {
				fname := fmt.Sprintf("p4_map_%s.json", safe(dir))
				if useCacheFor(4) && fileExists(filepath.Join(*outDir, fname)) {
					log.Printf("P4 map: using cache for %s", dir)
					var out t.P4Out
					readJSON(*outDir, fname, &out)
					mu.Lock()
					mapBatches = append(mapBatches, out)
					mu.Unlock()
					return nil
				}

				// collect up to 3 files under dir
				var fmList []scan.FileMeta
				for _, fm := range tree.Files {
					if filepath.Dir(fm.Path) == dir {
						fmList = append(fmList, fm)
					}
				}
				pick := fmList
				if len(pick) > 3 {
					pick = pick[:3]
				}

				ev := scan.BuildEvidence(tree, dir, pick)

				acquire()
				defer release()
				ctxMap := llm.WithPhase(gctx, "p4_map:"+safe(dir))
				out, err := (&pipeline.P4Map{LLM: llmCli}).Run(ctxMap, p3Out.Nodes, ev)
				if err != nil {
					log.Printf("P4 map error (%s): %v", dir, err)
					return nil // keep going
				}
				writeJSON(*outDir, fname, out)
				mu.Lock()
				mapBatches = append(mapBatches, out)
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			log.Printf("P4 map parallel: %v", err)
		}
	}

	// ------------------------- P4 Reduce -------------------------------------
	var p4Out t.P4Out
	if useCacheFor(4) && fileExists(filepath.Join(*outDir, "p4.json")) {
		log.Println("P4 reduce: using cache")
		readJSON(*outDir, "p4.json", &p4Out)
		// Optional: normalize cached output too
		// pipeline.PostProcessP4(&p4Out, p3Out.Nodes)
		// writeJSON(*outDir, "p4.json", p4Out)
	} else {
		ctxRed := llm.WithPhase(ctx, "p4_reduce")
		out, err := (&pipeline.P4Reduce{LLM: llmCli}).Run(ctxRed, mapBatches)
		if err != nil {
			log.Fatal(err)
		}
		p4Out = out
		// Normalize (rebuild edge IDs, drop dangling, fix artifact.where, dedupe)
		pipeline.PostProcessP4(&p4Out, p3Out.Nodes)
		writeJSON(*outDir, "p4.json", p4Out)
	}

	log.Println("analysis completed →", *outDir)
}

// writeJSON writes JSON without HTML escaping (keeps "->" as-is).
func writeJSON(dir, name string, v any) {
	b, err := jsonutil.MarshalNoEscapeIndent(v, "", "  ")
	if err != nil {
		// fallback to stdlib if anything goes wrong
		b2, _ := json.MarshalIndent(v, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, name), b2, 0o644)
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
}

func readJSON(dir, name string, v any) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		log.Fatalf("failed to read %s: %v", name, err)
	}
	// tolerant unmarshal (handles \u003e etc.)
	if err := jsonutil.Unmarshal(b, v); err != nil {
		log.Fatalf("failed to unmarshal %s: %v", name, err)
	}
}

func fileExists(path string) bool {
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return true
	}
	return false
}

// safe replaces slashes so the string can be used as a filename suffix.
func safe(s string) string {
	s = filepath.Clean(s)
	s = filepath.ToSlash(s)
	s = strings.ReplaceAll(s, "/", "_")
	if s == "." {
		return "root"
	}
	return s
}

// phaseIndex returns the numeric order of a phase name, or -1 if empty/invalid.
func phaseIndex(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "":
		return -1
	case "p0":
		return 0
	case "p1":
		return 1
	case "p2":
		return 2
	case "p3":
		return 3
	case "p4":
		return 4
	default:
		log.Fatalf("invalid --phase: %s (use p0|p1|p2|p3|p4)", p)
		return -1
	}
}

// PromptSaver hooks are used by llm.WithHook to persist prompts/responses per phase.
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
