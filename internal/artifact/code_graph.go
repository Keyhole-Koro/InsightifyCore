package artifact

// CodeGraphIn consumes decoded C2 dependencies to produce a simplified directed graph.
type CodeGraphIn struct {
	Repo         string         `json:"repo"`
	Dependencies []Dependencies `json:"dependencies"`
}

// CodeGraphOut represents a dependency graph with fully materialized node metadata.
type CodeGraphOut struct {
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
