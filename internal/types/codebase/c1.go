package codebase

import (
	mainline "insightify/internal/types/mainline"
	"insightify/internal/wordidx"
)

// C1In carries inputs to build a dependency graph from extractor specs.
type C1In struct {
	Repo      string                   `json:"repo"`
	Roots     mainline.M0Out           `json:"roots"`
	Specs     map[string]ExtractorSpec `json:"specs"`
	WordIndex wordidx.AggIndex         `json:"word_index"`
}

// C1Out is a minimal dependency graph.
type C1Out struct {
	PossibleDependencies []FileWithDependency `json:"possible_dependencies,omitempty"`
}

type FileWithDependency struct {
	Path     string   `json:"path"`
	Language string   `json:"language,omitempty"`
	Ext      string   `json:"ext,omitempty"`
	Requires []string `json:"requires"`
}

// ImportStatementRange identifies a contiguous range of lines that likely contain
// import or include statements in a given file.
type ImportStatementRange struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}
