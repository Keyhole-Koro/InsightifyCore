package types

// X1In carries inputs to build a dependency graph from extractor specs.
type X1In struct {
    Repo  string            `json:"repo"`
    Index []FileIndexEntry  `json:"index"`
    Specs []X0Spec          `json:"specs"`
}

// X1Edge represents a dependency from one file to a module/path (possibly resolved).
type X1Edge struct {
    From   string `json:"from"`              // repository-relative file path
    Module string `json:"module"`            // raw module text found (e.g., "./util", "react")
    To     string `json:"to,omitempty"`      // resolved repository-relative file path when found
    Reason string `json:"reason,omitempty"`  // e.g., "external module", "file found with .ts"
}

// X1Out is a minimal dependency graph.
type X1Out struct {
    Edges   []X1Edge `json:"edges"`
    Matches int      `json:"matches"`
    Files   int      `json:"files"`
}

