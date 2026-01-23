package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"insightify/internal/adaptors/snippet"
	"insightify/internal/pipeline/codebase"
	cb "insightify/internal/types/codebase"
)

// --------------------- snippet.collect ---------------------

type snippetCollectTool struct{ host Host }

func newSnippetCollectTool(h Host) *snippetCollectTool { return &snippetCollectTool{host: h} }

func (t *snippetCollectTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "snippet.collect",
		Description: "Collect related code snippets for identifiers (uses existing codebase artifacts).",
	}
}

type snippetCollectInput struct {
	Seeds     []snippet.Identifier `json:"seeds"`
	MaxTokens int                  `json:"max_tokens"`
}

type snippetCollectOutput struct {
	Snippets []snippetOut `json:"snippets"`
}

type snippetOut struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Tokens   int    `json:"tokens"`
	Code     string `json:"code"`
}

func (t *snippetCollectTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in snippetCollectInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}
	if len(in.Seeds) == 0 {
		return nil, fmt.Errorf("snippet.collect: seeds required")
	}
	c4, err := loadC4Out(t.host)
	if err != nil {
		return nil, err
	}
	provider := codebase.NewC4SnippetProvider(t.host.RepoRoot, c4)
	outSnips, err := provider.Collect(ctx, snippet.Query{
		Seeds:     in.Seeds,
		MaxTokens: in.MaxTokens,
	})
	if err != nil {
		return nil, err
	}
	out := snippetCollectOutput{Snippets: make([]snippetOut, 0, len(outSnips))}
	for _, s := range outSnips {
		out.Snippets = append(out.Snippets, snippetOut{
			Path:     s.Identifier.Path,
			Name:     s.Identifier.Name,
			Provider: s.Provider,
			Tokens:   s.Tokens,
			Code:     s.Code,
		})
	}
	return json.Marshal(out)
}

func loadC4Out(h Host) (cb.C4Out, error) {
	fs := h.ArtifactFS
	if fs == nil {
		return cb.C4Out{}, fmt.Errorf("snippet.collect: artifact fs not configured")
	}
	b, err := fs.SafeReadFile(filepath.Join(".", "c4.json"))
	if err != nil {
		return cb.C4Out{}, fmt.Errorf("snippet.collect: read c4.json: %w", err)
	}
	var out cb.C4Out
	if err := json.Unmarshal(b, &out); err != nil {
		return cb.C4Out{}, fmt.Errorf("snippet.collect: decode c4.json: %w", err)
	}
	return out, nil
}
