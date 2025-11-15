package scan

import (
	"bufio"
	"path/filepath"
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
	abs := filepath.Join(repo, filepath.FromSlash(in.FilePath))
	f, err := safeFS().SafeOpen(abs)
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
