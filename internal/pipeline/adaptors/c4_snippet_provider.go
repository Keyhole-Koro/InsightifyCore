package adaptors

import (
	"insightify/internal/scan"
	cb "insightify/internal/types/codebase"
)

// C4SnippetProvider adapts C4Out into a SnippetProvider.
type C4SnippetProvider struct {
	provider SnippetProvider
}

// NewC4SnippetProvider builds a provider from a C4Out artifact.
func NewC4SnippetProvider(repoRoot string, c4 cb.C4Out) *C4SnippetProvider {
	idx := newC4Index(c4)
	return &C4SnippetProvider{
		provider: SnippetProvider{
			RepoRoot: repoRoot,
			Index:    idx,
		},
	}
}

// WithMaxDepth sets recursion depth for dependency traversal.
func (p *C4SnippetProvider) WithMaxDepth(depth int) *C4SnippetProvider {
	cp := *p
	cp.provider.MaxDepth = depth
	return &cp
}

// Collect retrieves snippets for the seeds and their requirements.
func (p *C4SnippetProvider) Collect(seeds []IdentifierSelector) ([]scan.Snippet, map[IdentifierSelector]error) {
	return p.provider.Collect(seeds)
}

// --- internal C4 index ---

type c4Index struct {
	data map[string]map[string]cb.IdentifierSignal // path -> name -> signal
}

func newC4Index(c4 cb.C4Out) *c4Index {
	idx := make(map[string]map[string]cb.IdentifierSignal)
	for _, file := range c4.Files {
		path := normalizePath(file.Path)
		if path == "" {
			continue
		}
		for _, id := range file.Identifiers {
			if id.Name == "" {
				continue
			}
			if _, ok := idx[path]; !ok {
				idx[path] = make(map[string]cb.IdentifierSignal)
			}
			idx[path][id.Name] = id
		}
	}
	return &c4Index{data: idx}
}

func (c *c4Index) Get(sel IdentifierSelector) (SymbolRecord, bool) {
	path := normalizePath(sel.Path)
	byPath, ok := c.data[path]
	if !ok {
		return SymbolRecord{}, false
	}
	sig, ok := byPath[sel.Identifier]
	if !ok {
		return SymbolRecord{}, false
	}

	rec := SymbolRecord{
		Selector: IdentifierSelector{
			Path:       path,
			Identifier: sel.Identifier,
		},
		Lines:   sig.Lines,
		Summary: sig.Summary,
		Role:    sig.Role,
	}
	if sig.Scope.Level != "" {
		rec.Scope = sig.Scope.Level
		rec.Access = sig.Scope.Access
	}
	for _, req := range sig.Requires {
		rec.Requires = append(rec.Requires, Requirement{
			Selector: IdentifierSelector{
				Path:       normalizePath(req.Path),
				Identifier: req.Identifier,
			},
			Origin: req.Origin,
		})
	}
	return rec, true
}
