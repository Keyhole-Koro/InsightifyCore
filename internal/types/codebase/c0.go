package codebase

import (
	types "insightify/internal/types"
	mainline "insightify/internal/types/mainline"
)

type C0In struct {
	Repo      string           `json:"repo"`
	ExtCounts []types.ExtCount `json:"ext_counts"`
	Roots     mainline.M0Out   `json:"roots"`
}

type C0Out struct {
	FamilyKeys map[string][]string      `json:"familyKeys"`
	Specs      map[string]ExtractorSpec `json:"specs"`
}

type ExtractorSpec struct {
	Ext                 []string       `json:"ext"`      // e.g. [".ts",".js"]
	Language            []Language     `json:"language"` // e.g. ["TypeScript","JavaScript"]
	Rules               Rules          `json:"rules"`
	CommentLinePattern  []string       `json:"comment_line_pattern"`
	CommentBlockPattern []string       `json:"comment_block_pattern"`
	NormalizeHints      NormalizeHints `json:"normalize_hints"`
}

// C0Spec is an alias for ExtractorSpec for compatibility with older code.
type C0Spec = ExtractorSpec

type Language struct {
	Name string   `json:"name"` // e.g. "TypeScript"
	Ext  []string `json:"ext"`  // e.g. [".ts"]
}

type Rules struct {
	Keywords  []string `json:"keywords"`   // tokens like "import","from","require"
	PathSplit []string `json:"path_split"` // separators to split around the path
}

type NormalizeHints struct {
	Alias []AliasPair `json:"alias"` // optional alias normalization map
}

type AliasPair struct {
	Original   string `json:"original"`
	Normalized string `json:"normalized"`
}
