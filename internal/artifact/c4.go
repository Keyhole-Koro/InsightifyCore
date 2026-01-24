package artifact

import (
	"insightify/internal/safeio"
)

// C4In drives identifier extraction tasks via the scheduler.
type C4In struct {
	Repo   string         `json:"repo"`
	RepoFS *safeio.SafeFS `json:"-"`
	Tasks  C3Out          `json:"tasks"`
}

type C4Out struct {
	Repo  string             `json:"repo"`
	Files []IdentifierReport `json:"files"`
}
