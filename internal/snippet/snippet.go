package snippet

import (
	"context"

	"insightify/internal/artifact"
)

// Identifier points to a symbol by path+name.
type Identifier struct {
	Path string
	Name string
}

// Query describes a traversal request starting from seed identifiers.
type Query struct {
	Seeds       []Identifier     // starting identifiers
	MaxTokens   int              // token budget; <=0 means unlimited
	CountTokens func(string) int // token counter; defaults to len([]rune(code))
}

// RelatedSnippet carries the code slice and related metadata.
type RelatedSnippet struct {
	Identifier Identifier
	Report     artifact.IdentifierReport
	Signal     artifact.IdentifierSignal
	Code       string
	Tokens     int
	Provider   string // e.g. "c4"
}

// Provider resolves identifiers to code snippets, possibly traversing dependencies.
type Provider interface {
	Collect(ctx context.Context, q Query) ([]RelatedSnippet, error)
}