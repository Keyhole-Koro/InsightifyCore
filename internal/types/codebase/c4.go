package codebase

import "insightify/internal/safeio"

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

type IdentifierReport struct {
	Path        string             `json:"path"`
	Notes       []string           `json:"notes,omitempty"`
	Identifiers []IdentifierSignal `json:"identifiers"`
}

type IdentifierSignal struct {
	Name     string                  `json:"name"`
	Role     string                  `json:"role,omitempty"`
	Lines    [2]int                  `json:"lines"`
	Summary  string                  `json:"summary,omitempty"`
	Scope    IdentifierScope         `json:"scope"`
	Requires []IdentifierRequirement `json:"requires,omitempty"`
}

type IdentifierScope struct {
	Level  string `json:"level"`            // e.g. local|file|module|repository
	Access string `json:"access,omitempty"` // describes visibility
	Notes  string `json:"notes,omitempty"`
}

type IdentifierRequirement struct {
	Path       string `json:"path"`             // target file path
	Identifier string `json:"identifier"`       // required identifier name
	Origin     string `json:"origin,omitempty"` // user|library|runtime|vendor|stdlib|framework
}
