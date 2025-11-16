package codebase

import "insightify/internal/safeio"

// C3In converts the dependency graph into scheduler-friendly tasks.
type C3In struct {
	Repo        string         `json:"repo"`
	RepoFS      *safeio.SafeFS `json:"-"`
	Graph       C2Out          `json:"graph"`
	CapPerChunk int            `json:"cap_per_chunk"`
}

// C3Out encodes nodes with weights and adjacency for scheduler input.
type C3Out struct {
	Repo        string   `json:"repo"`
	CapPerChunk int      `json:"cap_per_chunk"`
	Nodes       []C3Node `json:"nodes"`
	Adjacency   [][]int  `json:"adjacency"`
}

type C3Node struct {
	ID       int    `json:"id"`
	Path     string `json:"path"`
	TaskType string `json:"task_type"`
	Weight   int    `json:"weight"`
}
