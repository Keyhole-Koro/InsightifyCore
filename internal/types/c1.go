package types

// C1In carries inputs to build a dependency graph from extractor specs.
type C1In struct {
	Repo  string          `json:"repo"`
	Roots M0Out           `json:"roots"`
	Specs []ExtractorSpec `json:"specs"`
}

// C1Out is a minimal dependency graph.
type C1Out struct {
	ImportStatementRanges []PerExtImportStatement `json:"import_statement_ranges,omitempty"`
}

type PerExtImportStatement struct {
	Ext       string                 `json:"ext,omitempty"`
	StmtRange []ImportStatementRange `json:"stmt_range,omitempty"`
}

type FileWords struct {
	Path string `json:"path"`
	Pos  struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"words"` // Positions of Indexer.go
}

type ImportStatementRange struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}
