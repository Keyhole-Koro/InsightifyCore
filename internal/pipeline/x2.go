package pipeline

import (
    "path/filepath"
    "sort"
    "strings"

    t "insightify/internal/types"
)

// X2 sorts files by fewest dependencies and emits detailed per-file dependency objects.
type X2 struct{}

func (X2) Run(in t.X2In) (t.X2Out, error) {
    // Build set of all candidate files and lookup maps from index
    all := map[string]struct{}{}
    sizes := map[string]int64{}
    exts := map[string]string{}
    langs := map[string]string{}
    for _, it := range in.Index {
        p := filepath.ToSlash(it.Path)
        all[p] = struct{}{}
        sizes[p] = it.Size
        e := strings.ToLower(filepath.Ext(p))
        if it.Ext != "" { e = strings.ToLower(it.Ext) }
        exts[p] = e
        if it.Language != "" {
            langs[p] = it.Language
        } else {
            langs[p] = languageForExt(e)
        }
    }

    // Forward and reverse internal dependency maps
    fwd := map[string]map[string]struct{}{}
    rev := map[string]map[string]struct{}{}
    for _, e := range in.Graph.Edges {
        from := filepath.ToSlash(strings.TrimSpace(e.From))
        to := filepath.ToSlash(strings.TrimSpace(e.To))
        if from == "" || to == "" { // only consider resolved internal edges
            continue
        }
        if _, ok := fwd[from]; !ok { fwd[from] = map[string]struct{}{} }
        if _, ok := rev[to]; !ok { rev[to] = map[string]struct{}{} }
        fwd[from][to] = struct{}{}
        rev[to][from] = struct{}{}
    }

    // Build nodes list for sorting (by fewest internal deps)
    var nodes []t.X2Node
    for p := range all {
        n := 0
        if m, ok := fwd[p]; ok { n = len(m) }
        nodes = append(nodes, t.X2Node{Path: p, InternalDeps: n})
    }
    sort.Slice(nodes, func(i, j int) bool {
        if nodes[i].InternalDeps != nodes[j].InternalDeps {
            return nodes[i].InternalDeps < nodes[j].InternalDeps
        }
        return nodes[i].Path < nodes[j].Path
    })

    // Build detailed FileWithDependency objects in the same order
    files := make([]t.FileWithDependency, 0, len(nodes))
    for _, n := range nodes {
        req := setKeys(fwd[n.Path])
        rby := setKeys(rev[n.Path])
        sort.Strings(req)
        sort.Strings(rby)
        files = append(files, t.FileWithDependency{
            Path:       n.Path,
            Size:       sizes[n.Path],
            Language:   langs[n.Path],
            Ext:        exts[n.Path],
            Requires:   req,
            RequiredBy: rby,
        })
    }

    return t.X2Out{Sorted: nodes, Files: files}, nil
}

func languageForExt(ext string) string {
    switch strings.ToLower(ext) {
    case ".ts", ".tsx":
        return "TypeScript"
    case ".js", ".jsx", ".mjs", ".cjs":
        return "JavaScript"
    case ".go":
        return "Go"
    case ".py":
        return "Python"
    case ".rs":
        return "Rust"
    case ".java":
        return "Java"
    case ".kt":
        return "Kotlin"
    case ".swift":
        return "Swift"
    case ".c", ".h":
        return "C"
    case ".cpp", ".hpp", ".cc", ".hh":
        return "C++"
    case ".sh":
        return "Shell"
    default:
        return ""
    }
}

func setKeys(m map[string]struct{}) []string {
    if len(m) == 0 { return []string{} }
    out := make([]string, 0, len(m))
    for k := range m { out = append(out, k) }
    return out
}
