package main

import (
    "context"
    "bytes"
    "time"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"

    "github.com/joho/godotenv"

    "insightify/internal/llm"
    "insightify/internal/pipeline"
    "insightify/internal/scan"
    t "insightify/internal/types"
)

type PromptSaver struct{ Dir string }

func (p *PromptSaver) Before(ctx context.Context, phase, prompt string, input any) {
    if phase == "" { phase = "unknown" }
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
    if phase == "" { phase = "unknown" }
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


func main() {
    repo := flag.String("repo", "", "path to the repository root")
    model := flag.String("model", "gemini-2.5-flash", "Gemini model id")
    outDir := flag.String("out", "out", "output directory")
    phase := flag.String("phase", "", "start phase whose results and later phases should be taken from cache unless --force (p0|p1|p2|p3)")
    force := flag.Bool("force", false, "recompute specified phase and later phases, ignoring cache")
    flag.Parse()
    if *repo == "" { log.Fatal("--repo is required") }

    if err := os.MkdirAll(*outDir, 0o755); err != nil { log.Fatal(err) }

    // Load env; accept GEMINI_API_KEY or GOOGLE_API_KEY
    _ = godotenv.Load()
    apiKey := os.Getenv("GEMINI_API_KEY")
    if apiKey == "" { apiKey = os.Getenv("GOOGLE_API_KEY") }
    if apiKey == "" { log.Fatal("GEMINI_API_KEY or GOOGLE_API_KEY must be set") }

    ctx := context.Background()
    base, err := llm.NewGeminiClient(ctx, apiKey, *model); if err != nil { log.Fatal(err) }
    llmCli := llm.WithHook(base, &PromptSaver{Dir: *outDir})

    defer llmCli.Close()

    // Scan repo (always fresh; cheap & deterministic)
    tree, err := scan.Scan(*repo); if err != nil { log.Fatal(err) }
    log.Printf("scanned %d files in %s", len(tree.Files), *repo)

    // Light context for P0
    heads := scan.ExtractDocHeadings(*repo, tree)
    manifests := scan.CollectManifests(*repo, tree)

    // Cache policy: if --phase is set, phases >= that index use cache, unless --force
    phaseIdx := phaseIndex(*phase)
    useCacheFor := func(i int) bool { return phaseIdx >= 0 && i >= phaseIdx && !*force }

    // ------------------------- P0 -------------------------------------------
    var p0Out t.P0Out
    if useCacheFor(0) && fileExists(filepath.Join(*outDir, "p0.json")) {
        log.Println("P0: using cache")
        readJSON(*outDir, "p0.json", &p0Out)
    } else {
        log.Println("P0: computing…")
        p0 := pipeline.P0{LLM: llmCli}
        // Build a compact tree summary for P0 (path/ext/loc only; no mtime/size)
        treeSummary := make([]map[string]any, 0, len(tree.Files))
        for _, f := range tree.Files {
            treeSummary = append(treeSummary, map[string]any{"path": f.Path, "ext": f.Ext, "loc": f.LOC})
        }
        ctxP0 := llm.WithPhase(ctx, "p0")
        p0Out, err = p0.Run(ctxP0, treeSummary, heads, manifests, nil)
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
            if fm, ok := tree.FindByPath(d.Path); ok { docSnips = append(docSnips, scan.Extract(tree, fm)) }
        }
        for _, e := range p0Out.EntryPoints {
            if fm, ok := tree.FindByPath(e.Path); ok { entrySnips = append(entrySnips, scan.Extract(tree, fm)) }
        }
        p1 := pipeline.P1{LLM: llmCli}
        ctxP1 := llm.WithPhase(ctx, "p1")
        p1Out, err = p1.Run(ctxP1, docSnips, entrySnips, manifests); if err != nil { log.Fatal(err) }
        writeJSON(*outDir, "p1.json", p1Out)
    }

    // ------------------------- P2 -------------------------------------------
    var allP2 []t.P2Out
    p2 := pipeline.P2{LLM: llmCli}

    // dedupe dirs from read targets
    dirSet := map[string]bool{}
    for _, rt := range p1Out.ReadTargets { dirSet[filepath.Dir(rt.Path)] = true }

    for dir := range dirSet {
        f := fmt.Sprintf("p2_%s.json", safe(dir))
        if useCacheFor(2) && fileExists(filepath.Join(*outDir, f)) {
            log.Printf("P2: using cache for %s", dir)
            var out t.P2Out
            readJSON(*outDir, f, &out)
            allP2 = append(allP2, out)
            continue
        }
        // compute
        log.Printf("P2: computing %s…", dir)
        var fmList []scan.FileMeta
        for _, fm := range tree.Files { if filepath.Dir(fm.Path) == dir { fmList = append(fmList, fm) } }
        // pick up to 3 by simple priority (router/service/index get earlier in scan)
        var snips []scan.Snippet
        for i, fm := range fmList { if i >= 3 { break }; snips = append(snips, scan.Extract(tree, fm)) }
        ctxP2 := llm.WithPhase(ctx, "p2")
        out, err := p2.Run(ctxP2, dir, snips, p1Out.Taxonomy, p1Out.Glossary); if err != nil { log.Printf("P2 error (%s): %v", dir, err); continue }
        writeJSON(*outDir, f, out)
        allP2 = append(allP2, out)
    }

    // ------------------------- P3 -------------------------------------------
    var p3Out t.P3Out
    if useCacheFor(3) && fileExists(filepath.Join(*outDir, "p3.json")) {
        log.Println("P3: using cache")
        readJSON(*outDir, "p3.json", &p3Out)
    } else {
        log.Println("P3: computing…")
        p3 := pipeline.P3{LLM: llmCli}
        var err error
        ctxP3 := llm.WithPhase(ctx, "p3")
        p3Out, err = p3.Run(ctxP3, allP2, p1Out.Glossary, p1Out.Taxonomy); if err != nil { log.Fatal(err) }
        writeJSON(*outDir, "p3.json", p3Out)
    }

    log.Println("analysis completed →", *outDir)
}

func writeJSON(dir, name string, v any) {
    b, _ := json.MarshalIndent(v, "", "  ")
    _ = os.WriteFile(filepath.Join(dir, name), b, 0o644)
}

func readJSON(dir, name string, v any) {
    b, err := os.ReadFile(filepath.Join(dir, name))
    if err != nil { log.Fatalf("failed to read %s: %v", name, err) }
    if err := json.Unmarshal(b, v); err != nil { log.Fatalf("failed to unmarshal %s: %v", name, err) }
}

func fileExists(path string) bool {
    if fi, err := os.Stat(path); err == nil && !fi.IsDir() { return true }
    return false
}

func safe(s string) string {
    s = filepath.Clean(s)
    s = filepath.ToSlash(s)
    s = strings.ReplaceAll(s, "/", "_")
    if s == "." { return "root" }
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
    default:
        log.Fatalf("invalid --phase: %s (use p0|p1|p2|p3)", p)
        return -1
    }
}
