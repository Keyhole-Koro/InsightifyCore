package artifact

// C2In consumes decoded C1 dependencies to produce a simplified directed graph.
type C2In struct {
	Repo         string         `json:"repo"`
	Dependencies []Dependencies `json:"dependencies"`
}

// C2Out represents a dependency graph with fully materialized node metadata.
type C2Out struct {
	Repo  string          `json:"repo"`
	Graph DependencyGraph `json:"graph"`
}

type DependencyGraph struct {
	Nodes     []DependencyNode `json:"nodes"`
	Adjacency [][]int          `json:"adjacency"`
}

type DependencyNode struct {
	ID   int     `json:"id"`
	File FileRef `json:"file"`
}
