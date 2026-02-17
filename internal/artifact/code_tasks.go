package artifact

import "insightify/internal/common/safeio"

// CodeTasksIn converts the dependency graph into scheduler-friendly tasks.
type CodeTasksIn struct {
	Repo        string          `json:"repo"`
	RepoFS      *safeio.SafeFS  `json:"-"`
	Graph       DependencyGraph `json:"graph"`
	CapPerChunk int             `json:"cap_per_chunk"`
}

// CodeTasksOut encodes nodes with weights and adjacency for scheduler input.
type CodeTasksOut struct {
	Repo        string   `json:"repo"`
	CapPerChunk int      `json:"cap_per_chunk"`
	Nodes       []CodeTasksNode `json:"nodes"`
	Adjacency   [][]int  `json:"adjacency"`
}

type CodeTasksNode struct {
	ID       int     `json:"id"`
	Path     string  `json:"path,omitempty"` // legacy compatibility; prefer File.Path
	File     FileRef `json:"file"`
	TaskType string  `json:"task_type"`
	Weight   int     `json:"weight"`
}
