package codebase

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"insightify/internal/artifact"
	"insightify/internal/common/snippet"
)

// CodeSymbolsSnippetProvider implements snippet.Provider backed by a CodeSymbolsOut value.
type CodeSymbolsSnippetProvider struct {
	out      artifact.CodeSymbolsOut
	repoRoot string
}

// NewCodeSymbolsSnippetProvider constructs a provider that serves queries from the given CodeSymbolsOut.
func NewCodeSymbolsSnippetProvider(repoRoot string, out artifact.CodeSymbolsOut) *CodeSymbolsSnippetProvider {
	return &CodeSymbolsSnippetProvider{out: out, repoRoot: repoRoot}
}

// Collect returns snippets for seeds and their requires in BFS order until MaxTokens is reached.
func (p *CodeSymbolsSnippetProvider) Collect(ctx context.Context, q snippet.Query) ([]snippet.RelatedSnippet, error) {
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

	index := buildCodeSymbolsIndex(p.out.Files)
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
			Provider:   "codeSymbols",
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

type codeSymbolsIndex struct {
	byPath map[string]artifact.IdentifierReport
}

func buildCodeSymbolsIndex(files []artifact.IdentifierReport) codeSymbolsIndex {
	m := make(map[string]artifact.IdentifierReport, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return codeSymbolsIndex{byPath: m}
}

func (idx codeSymbolsIndex) lookup(path, name string) (artifact.IdentifierReport, artifact.IdentifierSignal, bool) {
	rep, ok := idx.byPath[path]
	if !ok {
		return artifact.IdentifierReport{}, artifact.IdentifierSignal{}, false
	}
	for _, sig := range rep.Identifiers {
		if sig.Name == name {
			return rep, sig, true
		}
	}
	return artifact.IdentifierReport{}, artifact.IdentifierSignal{}, false
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