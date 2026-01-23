package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"insightify/internal/wordidx"
)

// --------------------- wordidx.search ---------------------

type wordIdxSearchTool struct{ host Host }

func newWordIdxSearchTool(h Host) *wordIdxSearchTool { return &wordIdxSearchTool{host: h} }

func (t *wordIdxSearchTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "wordidx.search",
		Description: "Search for exact word occurrences under repo roots using the word index.",
	}
}

type wordIdxInput struct {
	Roots      []string `json:"roots"`
	Word       string   `json:"word"`
	AllowExt   []string `json:"allow_ext"`
	MaxResults int      `json:"max_results"`
}

type wordIdxOutput struct {
	Matches []wordMatch `json:"matches"`
}

type wordMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

func (t *wordIdxSearchTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in wordIdxInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}
	if len(in.Roots) == 0 || strings.TrimSpace(in.Word) == "" {
		return nil, fmt.Errorf("wordidx.search: roots and word required")
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 200
	}
	roots := make([]string, 0, len(in.Roots))
	for _, r := range in.Roots {
		roots = append(roots, resolveRepoPath(t.host.RepoRoot, r))
	}
	builder := wordidx.New().Root(roots...).FS(t.host.RepoFS)
	if len(in.AllowExt) > 0 {
		exts := make([]string, 0, len(in.AllowExt))
		for _, e := range in.AllowExt {
			e = strings.ToLower(strings.TrimSpace(e))
			e = strings.TrimPrefix(e, ".")
			if e != "" {
				exts = append(exts, e)
			}
		}
		if len(exts) > 0 {
			builder = builder.Allow(exts...)
		}
	}
	agg := builder.Start(ctx)
	matches := agg.Find(ctx, in.Word)
	out := wordIdxOutput{Matches: make([]wordMatch, 0, len(matches))}
	for _, m := range matches {
		out.Matches = append(out.Matches, wordMatch{Path: m.FilePath, Line: m.Line, Column: 0})
		if len(out.Matches) >= in.MaxResults {
			break
		}
	}
	return json.Marshal(out)
}
