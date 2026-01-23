package codebase

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"insightify/internal/snippet"
	cb "insightify/internal/types/codebase"
)

// C4SnippetProvider implements snippet.Provider backed by a C4Out value.
type C4SnippetProvider struct {
	out      cb.C4Out
	repoRoot string
}

// NewC4SnippetProvider constructs a provider that serves queries from the given C4Out.
func NewC4SnippetProvider(repoRoot string, out cb.C4Out) *C4SnippetProvider {
	return &C4SnippetProvider{out: out, repoRoot: repoRoot}
}

// Collect returns snippets for seeds and their requires in BFS order until MaxTokens is reached.
func (p *C4SnippetProvider) Collect(ctx context.Context, q snippet.Query) ([]snippet.RelatedSnippet, error) {
	countFn := q.CountTokens
	if countFn == nil {
		countFn = func(s string) int { return len([]rune(s)) }
	}
	maxTokens := q.MaxTokens

	type entry struct {
		path string
		name string
	}
	queue := make([]entry, 0, len(q.Seeds))
	for _, s := range q.Seeds {
		queue = append(queue, entry{path: s.Path, name: s.Name})
	}

	index := buildC4Index(p.out.Files)
	visited := make(map[string]struct{})
	var results []snippet.RelatedSnippet
	used := 0

	for len(queue) > 0 {
		// context check
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		cur := queue[0]
		queue = queue[1:]
		key := cur.path + "#" + cur.name
		if _, ok := visited[key]; ok {
			continue
		}
		visited[key] = struct{}{}

		report, sig, ok := index.lookup(cur.path, cur.name)
		if !ok || sig.Lines[0] <= 0 || sig.Lines[1] <= 0 {
			continue
		}

		code, err := readSnippetFile(p.repoRoot, cur.path, sig.Lines[0], sig.Lines[1])
		if err != nil {
			continue
		}
		toks := countFn(code)
		if maxTokens > 0 && used+toks > maxTokens {
			break
		}
		used += toks
		results = append(results, snippet.RelatedSnippet{
			Identifier: snippet.Identifier{Path: cur.path, Name: cur.name},
			Report:     report,
			Signal:     sig,
			Code:       code,
			Tokens:     toks,
			Provider:   "c4",
		})

		for _, req := range sig.Requires {
			if req.Path == "" || req.Identifier == "" {
				continue
			}
			queue = append(queue, entry{path: req.Path, name: req.Identifier})
		}
	}

	return results, nil
}

type c4Index struct {
	byPath map[string]cb.IdentifierReport
}

func buildC4Index(files []cb.IdentifierReport) c4Index {
	m := make(map[string]cb.IdentifierReport, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return c4Index{byPath: m}
}

func (idx c4Index) lookup(path, name string) (cb.IdentifierReport, cb.IdentifierSignal, bool) {
	rep, ok := idx.byPath[path]
	if !ok {
		return cb.IdentifierReport{}, cb.IdentifierSignal{}, false
	}
	for _, sig := range rep.Identifiers {
		if sig.Name == name {
			return rep, sig, true
		}
	}
	return cb.IdentifierReport{}, cb.IdentifierSignal{}, false
}

func readSnippetFile(repoRoot, relPath string, start, end int) (string, error) {
	if start <= 0 {
		start = 1
	}
	if end > 0 && end < start {
		start, end = end, start
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	f, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", relPath, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	var data []byte
	for sc.Scan() {
		line++
		if line < start {
			continue
		}
		if end > 0 && line > end {
			break
		}
		data = append(data, sc.Bytes()...)
		data = append(data, '\n')
	}
	return string(data), nil
}
