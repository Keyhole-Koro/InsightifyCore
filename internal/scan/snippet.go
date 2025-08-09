package scan

import (
    "os"
    "path/filepath"
    "regexp"
    "strings"

    t "insightify/internal/types"
)

type Snippet struct {
    Path string `json:"path"`
    Mode string `json:"mode"` // full|head|struct
    Text string `json:"text"`
}

func Extract(rt RepoTree, fm FileMeta) Snippet {
    fullPath := filepath.Join(rt.Root, fm.Path)
    b, _ := os.ReadFile(fullPath)
    src := string(b)
    if strings.HasSuffix(strings.ToLower(fm.Path), ".md") {
        src = cleanMarkdownImages(src)
    }
    switch {
    case fm.LOC <= 200:
        return Snippet{Path: fm.Path, Mode: "full", Text: src}
    case fm.LOC <= 800:
        return Snippet{Path: fm.Path, Mode: "head", Text: headN(src, 3000)}
    default:
        return Snippet{Path: fm.Path, Mode: "struct", Text: structural(src)}
    }
}

func headN(s string, n int) string { if len(s) < n { return s }; return s[:n] }

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

// remove inline images from markdown (e.g., ![alt](url) or <img ...>)
func cleanMarkdownImages(s string) string {
    lines := strings.Split(s, "\n")
    kept := make([]string, 0, len(lines))
    for _, l := range lines {
        trim := strings.TrimSpace(l)
        if strings.HasPrefix(trim, "![") { continue }
        if strings.Contains(trim, "<img") { continue }
        if strings.Contains(trim, "data:image/") { continue }
        kept = append(kept, l)
    }
    return strings.Join(kept, "\n")
}


func max(a, b int) int { if a > b { return a }; return b }
func min(a, b int) int { if a < b { return a }; return b }

func BuildEvidence(rt RepoTree, dir string, files []FileMeta) t.P4Evidence {
	ev := t.P4Evidence{Dir: dir}

	reBind := regexp.MustCompile(`(?m)\b(import|require|using|include|package)\b[^\n]*`)
	reInvoke := regexp.MustCompile(`(?m)\b([A-Za-z_][A-Za-z0-9_\.]+)\s*\(`)
	reIO := regexp.MustCompile(`(?is)\b(select|insert|update|delete)\b|https?://|(?i)\bfrom\b\s+['"][^'"]+['"]|queue|topic|subject`)
	reDecl := regexp.MustCompile(`(?m)\b(class|interface|struct|module|package)\b\s+[A-Za-z0-9_\.]+`)
	reAnno := regexp.MustCompile(`(?m)@\w+|\[[A-Za-z]+\]`)

	for _, fm := range files {
		if filepath.Dir(fm.Path) != dir {
			continue
		}
		sn := Extract(rt, fm)
		head := sn.Text

		for _, m := range reBind.FindAllString(head, -1) {
			ev.Signals = append(ev.Signals, t.Signal{Kind: "bind", File: fm.Path, Attrs: map[string]string{"raw": strings.TrimSpace(m)}})
		}
		for _, m := range reInvoke.FindAllStringSubmatch(head, -1) {
			if len(m) > 1 {
				ev.Signals = append(ev.Signals, t.Signal{Kind: "invoke", File: fm.Path, Attrs: map[string]string{"callee": m[1]}})
			}
		}
		for _, m := range reIO.FindAllString(head, -1) {
			ev.Signals = append(ev.Signals, t.Signal{Kind: "io", File: fm.Path, Attrs: map[string]string{"surface": strings.TrimSpace(m)}})
		}
		for _, m := range reDecl.FindAllString(head, -1) {
			ev.Signals = append(ev.Signals, t.Signal{Kind: "declare", File: fm.Path, Attrs: map[string]string{"raw": strings.TrimSpace(m)}})
		}
		for _, m := range reAnno.FindAllString(head, -1) {
			ev.Signals = append(ev.Signals, t.Signal{Kind: "annotate", File: fm.Path, Attrs: map[string]string{"raw": strings.TrimSpace(m)}})
		}
		text := head
		if len(text) > 800 {
			text = text[:800]
		}
		ev.Signals = append(ev.Signals, t.Signal{Kind: "file_head", File: fm.Path, Text: text})
	}
	return ev
}