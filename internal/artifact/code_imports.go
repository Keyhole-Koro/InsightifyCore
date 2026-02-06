package artifact

// CodeImportsIn carries inputs to build a dependency graph from extractor specs.
type CodeImportsIn struct {
	Repo     string       `json:"repo"`
	Roots    CodeRootsOut `json:"roots"`
	Families []FamilySpec `json:"families"`
}

// CodeImportsOut is a minimal dependency graph.
type CodeImportsOut struct {
	PossibleDependencies []Dependencies `json:"possible_dependencies,omitempty"`
}

type Dependencies struct {
	Repo    string             `json:"repo"`
	Roots   []string           `json:"roots"`
	Exts    []string           `json:"exts"`
	Family  string             `json:"family,omitempty"`
	SpecKey string             `json:"spec_key,omitempty"`
	Files   []SourceDependency `json:"files"`
}

type SourceDependency struct {
	File     FileRef   `json:"file"`
	Language string    `json:"language,omitempty"`
	Requires []FileRef `json:"requires"`
}

// ImportStatementRange identifies a contiguous range of lines that likely contain
// import or include statements in a given file.
type ImportStatementRange struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}
