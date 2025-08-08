package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "strings"
    "path/filepath"
    "github.com/joho/godotenv"

    "insightify/internal/llm"
    "insightify/internal/pipeline"
    "insightify/internal/scan"
    t "insightify/internal/types"
)

func main() {
    repo := flag.String("repo", "", "path to the repository root")
    model := flag.String("model", "gemini-2.5-flash", "Gemini model id")
    outDir := flag.String("out", "out", "output directory")
    phase := flag.String("phase", "", "start from specific phase: p0, p1, p2, p3")
    flag.Parse()
    if *repo == "" { log.Fatal("--repo is required") }
    if err := os.MkdirAll(*outDir, 0o755); err != nil { log.Fatal(err) }

    _ = godotenv.Load()
    apiKey := os.Getenv("GEMINI_API_KEY")
    if apiKey == "" { log.Fatal("GEMINI_API_KEY is not set") }

    ctx := context.Background()
    llmCli, err := llm.NewGeminiClient(ctx, apiKey, *model); if err != nil { log.Fatal(err) }

    tree, err := scan.Scan(*repo); if err != nil { log.Fatal(err) }
    log.Printf("scanned %d files in %s", len(tree.Files), *repo)

    var p0Out t.P0Out
    if *phase == "" || *phase == "p0" {
        log.Println("Phase 0")
        p0 := pipeline.P0{LLM: llmCli}
        p0Out, err = p0.Run(ctx, tree.Files, nil, nil, nil); if err != nil { log.Fatal(err) }
        writeJSON(*outDir, "p0.json", p0Out)
    } else {
        readJSON(*outDir, "p0.json", &p0Out)
    }

    var p1Out t.P1Out
    if *phase == "" || *phase == "p0" || *phase == "p1" {
        log.Println("Phase 1")
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
        p1Out, err = p1.Run(ctx, docSnips, entrySnips, nil); if err != nil { log.Fatal(err) }
        writeJSON(*outDir, "p1.json", p1Out)
    } else {
        readJSON(*outDir, "p1.json", &p1Out)
    }

    var allP2 []t.P2Out
    if *phase == "" || *phase == "p0" || *phase == "p1" || *phase == "p2" {
        log.Println("Phase 2")
        p2 := pipeline.P2{LLM: llmCli}
        for _, rt := range p1Out.ReadTargets {
            dir := filepath.Dir(rt.Path)
            var fmList []scan.FileMeta
            for _, f := range tree.Files {
                if filepath.Dir(f.Path) == dir { fmList = append(fmList, f) }
            }
            var snips []scan.Snippet
            for i, fm := range fmList {
                if i >= 3 { break }
                snips = append(snips, scan.Extract(tree, fm))
            }
            p2Out, err := p2.Run(ctx, dir, snips, p1Out.Taxonomy, p1Out.Glossary)
            if err != nil {
                log.Printf("P2 error (%s): %v", dir, err)
                continue
            }
            writeJSON(*outDir, fmt.Sprintf("p2_%s.json", safe(dir)), p2Out)
            allP2 = append(allP2, p2Out)
        }
    } else {
        for _, f := range p1Out.ReadTargets {
            var p2Out t.P2Out
            name := fmt.Sprintf("p2_%s.json", safe(filepath.Dir(f.Path)))
            readJSON(*outDir, name, &p2Out)
            allP2 = append(allP2, p2Out)
        }
    }

    if *phase == "" || *phase == "p0" || *phase == "p1" || *phase == "p2" || *phase == "p3" {
        log.Println("Phase 3")
        p3 := pipeline.P3{LLM: llmCli}
        p3Out, err := p3.Run(ctx, allP2, p1Out.Glossary, p1Out.Taxonomy)
        if err != nil { log.Fatal(err) }
        writeJSON(*outDir, "p3.json", p3Out)
    }

    log.Println("analysis completed →", *outDir)
}

func writeJSON(dir, name string, v any) {
    b, _ := json.MarshalIndent(v, "", "  ")
    os.WriteFile(filepath.Join(dir, name), b, 0o644)
}

func readJSON(dir, name string, v any) {
    b, err := os.ReadFile(filepath.Join(dir, name))
    if err != nil {
        log.Fatalf("failed to read %s: %v", name, err)
    }
    if err := json.Unmarshal(b, v); err != nil {
        log.Fatalf("failed to unmarshal %s: %v", name, err)
    }
}

func safe(s string) string {
    s = filepath.Clean(s)
    s = filepath.ToSlash(s)
    s = strings.ReplaceAll(s, "/", "_")
    return s
}
