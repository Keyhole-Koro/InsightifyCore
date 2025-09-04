package types

// X2In carries the file index and the dependency graph from X1.
type X2In struct {
    Index []FileIndexEntry `json:"index"`
    Graph X1Out            `json:"graph"`
}

// X2Node summarises a file and its dependency counts.
type X2Node struct {
    Path         string `json:"path"`
    InternalDeps int    `json:"internal_deps"`
}

// X2Out provides files sorted by fewest internal dependencies.
type X2Out struct {
    Sorted []X2Node `json:"sorted"`
    Files  []FileWithDependency `json:"files,omitempty"`
}

// FileWithDependency models a file and its internal dependency relationships.
type FileWithDependency struct {
    Path       string   `json:"path"`
    Size       int64    `json:"size,omitempty"`
    Language   string   `json:"language,omitempty"`
    Ext        string   `json:"ext,omitempty"`
    Requires   []string `json:"requires"`
    RequiredBy []string `json:"required_by"`
}
