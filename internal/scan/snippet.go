package scan

import (
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

type Snippet struct {
    Path string `json:"path"`
    Mode string `json:"mode"`
    Text string `json:"text"`
}

// Extract returns a snippet plan based on file metadata.
func Extract(rt RepoTree, fm FileMeta) Snippet {
    fullPath := filepath.Join(rt.Root, fm.Path)
    b, _ := os.ReadFile(fullPath)
    src := string(b)
    switch {
    case fm.LOC <= 200:
        return Snippet{Path: fm.Path, Mode: "full", Text: src}
    case fm.LOC <= 800:
        return Snippet{Path: fm.Path, Mode: "head", Text: headN(src, 3000)}
    default:
        return Snippet{Path: fm.Path, Mode: "struct", Text: structural(src)}
    }
}

func headN(s string, n int) string {
    if len(s) < n { return s }
    return s[:n]
}

func structural(s string) string {
    re := regexp.MustCompile(`(?i)\b(export|public|route|router\.|controller|service|interface|type)\b`)
    lines := strings.Split(s, "\n")
    var picked []string
    for i, l := range lines {
        if re.MatchString(l) {
            start := max(0, i-2); end := min(len(lines), i+3)
            picked = append(picked, strings.Join(lines[start:end], "\n"))
        }
        if len(strings.Join(picked, "\n")) > 3000 { break }
    }
    if len(picked) == 0 { return headN(s, 3000) }
    return strings.Join(picked, "\n")
}

func max(a, b int) int { if a > b { return a }; return b }
func min(a, b int) int { if a < b { return a }; return b }
