package types

// X2In carries the file index and the dependency graph from X1.
type X2In struct {
    Repo  string                     `json:"repo"`
    Stmts []PerExtImportStatement    `json:"stmt"`
}

// X2Out provides files sorted by fewest internal dependencies.
type X2Out struct {
	Dependencies []FileWithDependency `json:"dependencies"`
}

// FileWithDependency models a file and its internal dependency relationships.
type FileWithDependency struct {
	Path       string   `json:"path"`
	Language   string   `json:"language,omitempty"`
	Ext        string   `json:"ext,omitempty"`
	Requires   []string `json:"requires"`
	RequiredBy []string `json:"required_by"`
}
