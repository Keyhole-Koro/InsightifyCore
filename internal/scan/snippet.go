package scan

import (
    "bufio"
    "os"
    "path/filepath"
    "strings"
)

// SnippetInput defines a file slice to extract.
type SnippetInput struct {
	FilePath  string
	StartLine int
	EndLine   int
}

// Snippet carries the extracted slice text and its origin.
type Snippet struct {
	FilePath  string
	StartLine int
	EndLine   int
	Code      string
}

// ReadSnippet returns lines [start..end] (inclusive, 1-based). If end < start, it swaps them.
func ReadSnippet(repo string, in SnippetInput) (Snippet, error) {
    if in.StartLine <= 0 {
        in.StartLine = 1
    }
    if in.EndLine > 0 && in.EndLine < in.StartLine {
        in.StartLine, in.EndLine = in.EndLine, in.StartLine
    }
    abs := repoJoin(repo, in.FilePath)
    f, err := os.Open(abs)
    if err != nil {
        return Snippet{}, err
    }
	defer f.Close()

	var out Snippet
	out.FilePath = in.FilePath
	out.StartLine = in.StartLine
	out.EndLine = in.EndLine

	var b []byte
	scn := bufio.NewScanner(f)
	scn.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for scn.Scan() {
		line++
		if line < in.StartLine {
			continue
		}
		if in.EndLine > 0 && line > in.EndLine {
			break
		}
		b = append(b, scn.Bytes()...)
		b = append(b, '\n')
	}
	out.Code = string(b)
	return out, nil
}

// ChunkSnippets groups snippets into batches whose combined byte size does not exceed maxBytes.
// It counts code bytes plus a small overhead per snippet for prompt framing.
func ChunkSnippets(snips []Snippet, maxBytes int) [][]Snippet {
	if maxBytes <= 0 {
		maxBytes = 12_000
	}
	var res [][]Snippet
	var cur []Snippet
	curBytes := 0
	const perOverhead = 64 // rough JSON/prompt overhead per snippet
	for _, s := range snips {
		sz := len(s.Code) + perOverhead
		if sz > maxBytes {
			// Emit as its own chunk (truncated snippet handling could be added later)
			if len(cur) > 0 {
				res = append(res, cur)
				cur = nil
				curBytes = 0
			}
			res = append(res, []Snippet{s})
			continue
		}
		if curBytes+sz > maxBytes && len(cur) > 0 {
			res = append(res, cur)
			cur = nil
			curBytes = 0
		}
		cur = append(cur, s)
		curBytes += sz
	}
	if len(cur) > 0 {
		res = append(res, cur)
	}
	return res
}

// repoJoin joins a possibly repo-prefixed path with repo, avoiding double-prefixing.
// Accepts inputs like "src/a.ts" or "<repo>/src/a.ts" and returns an absolute path.
func repoJoin(repo, p string) string {
    p = strings.TrimSpace(p)
    if p == "" { return filepath.Clean(repo) }
    // If already absolute, use as-is
    if filepath.IsAbs(p) { return p }
    rp := filepath.ToSlash(p)
    rs := filepath.ToSlash(filepath.Clean(repo))
    rs = strings.TrimPrefix(rs, "./")
    // Strip leading "./" in rp for robustness
    rp = strings.TrimPrefix(rp, "./")
    // If rp erroneously starts with the repo segment, strip it
    if rs != "" && strings.HasPrefix(rp, rs+"/") {
        rp = strings.TrimPrefix(rp, rs+"/")
    }
    return filepath.Join(repo, filepath.FromSlash(rp))
}
