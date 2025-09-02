package pipeline

import (
    "bufio"
    "os"
    "path/filepath"
    "regexp"
    "strings"

    t "insightify/internal/types"
)

// X1 builds a dependency graph from extractor specs (X0 output) by scanning files.
// This pipeline does not use an LLM — it is entirely programmatic.
type X1 struct{}

func (X1) Run(in t.X1In) (t.X1Out, error) {
    // Build ext -> compiled rules
    type rule struct {
        re       *regexp.Regexp
        modGroup int // capture index for module; defaults to 1
        spec     t.X0Spec
    }
    rulesByExt := map[string][]rule{}
    for _, s := range in.Specs {
        // Determine module capture index per rule
        modIdx := 1
        for _, r := range s.Rules {
            if g, ok := r.Captures["module"]; ok {
                modIdx = g
            } else {
                modIdx = 1
            }
            // compile
            re, err := regexp.Compile(r.Pattern)
            if err != nil {
                continue // skip invalid
            }
            rulesByExt[strings.ToLower(s.Ext)] = append(rulesByExt[strings.ToLower(s.Ext)], rule{re: re, modGroup: modIdx, spec: s})
        }
    }

    // Quick index for existence checks
    filesSet := map[string]struct{}{}
    for _, f := range in.Index {
        filesSet[filepath.ToSlash(f.Path)] = struct{}{}
    }

    out := t.X1Out{}

    // Scan files
    for _, fi := range in.Index {
        ext := strings.ToLower(filepath.Ext(fi.Path))
        rs := rulesByExt[ext]
        if len(rs) == 0 {
            continue
        }
        full := filepath.Join(in.Repo, filepath.FromSlash(fi.Path))
        f, err := os.Open(full)
        if err != nil {
            continue
        }
        out.Files++
        sc := bufio.NewScanner(f)
        sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
        for sc.Scan() {
            line := sc.Text()
            // Try each rule for this extension
            for _, r := range rs {
                if m := r.re.FindStringSubmatch(line); m != nil {
                    mod := ""
                    if r.modGroup >= 0 && r.modGroup < len(m) {
                        mod = m[r.modGroup]
                    }
                    edge := t.X1Edge{From: filepath.ToSlash(fi.Path), Module: mod}
                    // Try to resolve to repo path for relative imports
                    if strings.HasPrefix(mod, ".") || strings.HasPrefix(mod, "/") {
                        // Resolve relative to file dir
                        baseDir := filepath.Dir(fi.Path)
                        cand := filepath.Clean(filepath.Join(baseDir, mod))
                        // Try with common extensions
                        if _, ok := filesSet[filepath.ToSlash(cand)]; ok {
                            edge.To = filepath.ToSlash(cand)
                            edge.Reason = "resolved exact"
                        } else {
                            for _, e := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
                                if _, ok := filesSet[filepath.ToSlash(cand+e)]; ok {
                                    edge.To = filepath.ToSlash(cand + e)
                                    edge.Reason = "resolved with ext " + e
                                    break
                                }
                                // index.ts pattern
                                ix := filepath.Join(cand, "index"+e)
                                if _, ok := filesSet[filepath.ToSlash(ix)]; ok {
                                    edge.To = filepath.ToSlash(ix)
                                    edge.Reason = "resolved index with ext " + e
                                    break
                                }
                            }
                        }
                    } else {
                        edge.Reason = "external module"
                    }
                    out.Edges = append(out.Edges, edge)
                    out.Matches++
                }
            }
        }
        _ = f.Close()
    }
    return out, nil
}

