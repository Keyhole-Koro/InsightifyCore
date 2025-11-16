package codebase

// C2In consumes decoded C1 dependencies to produce a simplified directed graph.
type C2In struct {
	Repo         string         `json:"repo"`
	Dependencies []Dependencies `json:"dependencies"`
}

// C2Out represents a directed dependency graph.
type C2Out struct {
	Repo       string                 `json:"repo"`
	Nodes      []string               `json:"nodes"`
	Edges      []DependencyEdge       `json:"edges"`
	Dependents []DependencyDependents `json:"dependents"`
}

type DependencyEdge struct {
	From string   `json:"from"`
	To   []string `json:"to"`
}

// DependencyDependents lists, for a given node, all direct dependents (reverse edges).
// This is convenient for topological ordering starting from leaf dependencies.
type DependencyDependents struct {
	Node       string   `json:"node"`
	Dependents []string `json:"dependents"`
}
