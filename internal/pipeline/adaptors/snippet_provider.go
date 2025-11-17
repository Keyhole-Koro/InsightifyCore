package adaptors

import (
	"fmt"
	"path/filepath"

	"insightify/internal/scan"
)

// IdentifierSelector identifies a specific symbol within a file.
type IdentifierSelector struct {
	Path       string
	Identifier string
}

// Requirement describes a dependency edge between two identifiers.
type Requirement struct {
	Selector IdentifierSelector
	Origin   string
}

// SymbolRecord captures snippet lookup metadata for an identifier.
type SymbolRecord struct {
	Selector IdentifierSelector
	Lines    [2]int
	Requires []Requirement
	Summary  string
	Role     string
	Scope    string
	Access   string
}

// SymbolIndex supplies symbol metadata for snippet resolution.
type SymbolIndex interface {
	Get(sel IdentifierSelector) (SymbolRecord, bool)
}

// SnippetProvider walks an index and returns snippets for seeds and their requirements.
type SnippetProvider struct {
	RepoRoot string
	Index    SymbolIndex
	MaxDepth int // optional recursion limit; defaults to 4 when <=0
}

// Collect returns snippets for the provided identifiers and their transitive requirements.
// Errors are reported per selector; duplicates are skipped via visited set.
func (p SnippetProvider) Collect(seeds []IdentifierSelector) ([]scan.Snippet, map[IdentifierSelector]error) {
	maxDepth := p.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 4
	}

	type queueItem struct {
		sel   IdentifierSelector
		depth int
	}
	var q []queueItem
	for _, s := range seeds {
		q = append(q, queueItem{sel: normalizeSel(s), depth: 0})
	}

	visited := make(map[IdentifierSelector]struct{})
	snips := []scan.Snippet{}
	errs := make(map[IdentifierSelector]error)

	for len(q) > 0 {
		item := q[0]
		q = q[1:]
		sel := item.sel
		if _, ok := visited[sel]; ok {
			continue
		}
		visited[sel] = struct{}{}

		rec, ok := p.Index.Get(sel)
		if !ok {
			errs[sel] = fmt.Errorf("identifier not found: %s#%s", sel.Path, sel.Identifier)
			continue
		}

		if rec.Lines[0] <= 0 || rec.Lines[1] <= 0 {
			errs[sel] = fmt.Errorf("missing line span")
		} else {
			snip, err := scan.ReadSnippet(p.RepoRoot, scan.SnippetInput{
				FilePath:  rec.Selector.Path,
				StartLine: rec.Lines[0],
				EndLine:   rec.Lines[1],
			})
			if err != nil {
				errs[sel] = fmt.Errorf("read snippet: %w", err)
			} else {
				snips = append(snips, snip)
			}
		}

		if item.depth >= maxDepth {
			continue
		}
		for _, req := range rec.Requires {
			if req.Selector.Path == "" || req.Selector.Identifier == "" {
				continue
			}
			q = append(q, queueItem{
				sel:   normalizeSel(req.Selector),
				depth: item.depth + 1,
			})
		}
	}

	return snips, errs
}

func normalizeSel(s IdentifierSelector) IdentifierSelector {
	return IdentifierSelector{
		Path:       normalizePath(s.Path),
		Identifier: s.Identifier,
	}
}

func normalizePath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}
