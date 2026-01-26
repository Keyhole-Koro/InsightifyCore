package artifact

import (
	"insightify/internal/safeio"
)

// CodeSymbolsIn drives identifier extraction tasks via the scheduler.
type CodeSymbolsIn struct {
	Repo   string         `json:"repo"`
	RepoFS *safeio.SafeFS `json:"-"`
	Tasks  CodeTasksOut          `json:"tasks"`
}

type CodeSymbolsOut struct {
	Repo  string             `json:"repo"`
	Files []IdentifierReport `json:"files"`
}
