package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"insightify/internal/artifact"
	"insightify/internal/workers/codebase"
	"insightify/internal/common/snippet"
)

// --------------------- snippet.collect ---------------------

type snippetCollectTool struct{ host Host }

func newSnippetCollectTool(h Host) *snippetCollectTool { return &snippetCollectTool{host: h} }

func (t *snippetCollectTool) Spec() artifact.ToolSpec {
	return artifact.ToolSpec{
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
	codeSymbols, err := loadCodeSymbolsOut(t.host)
	if err != nil {
		return nil, err
	}
	provider := codebase.NewCodeSymbolsSnippetProvider(t.host.RepoRoot, codeSymbols)
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

func loadCodeSymbolsOut(h Host) (artifact.CodeSymbolsOut, error) {
	fs := h.ArtifactFS
	if fs == nil {
		return artifact.CodeSymbolsOut{}, fmt.Errorf("snippet.collect: artifact fs not configured")
	}
	b, err := fs.SafeReadFile(filepath.Join(".", "code_symbols.json"))
	if err != nil {
		return artifact.CodeSymbolsOut{}, fmt.Errorf("snippet.collect: read code_symbols.json: %w", err)
	}
	var out artifact.CodeSymbolsOut
	if err := json.Unmarshal(b, &out); err != nil {
		return artifact.CodeSymbolsOut{}, fmt.Errorf("snippet.collect: decode code_symbols.json: %w", err)
	}
	return out, nil
}