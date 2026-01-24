package artifact

import "encoding/json"

// Generic evidence reference with optional line range.
type EvidenceRef struct {
	Path  string  `json:"path"`
	Lines *[2]int `json:"lines"` // nil means unknown/unspecified lines
}

// OpenedFile carries a file path and its textual content.
type OpenedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FocusQuestion represents a single confirmation target.
type FocusQuestion struct {
	Path     string `json:"path,omitempty"` // optional: where this question points to
	Question string `json:"question"`       // natural language question
}

// FileIndexEntry is a minimal index row for search hints.
type FileIndexEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size,omitempty"`
	Language string `json:"language,omitempty"`
	Kind     string `json:"kind,omitempty"` // code|config|doc|test|asset|other
	Ext      string `json:"ext,omitempty"`
}

// MDDoc holds extracted markdown text (images omitted).
type MDDoc struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

type ExtCount struct {
	Ext   string `json:"ext"`   // e.g. ".js"
	Count int    `json:"count"` // frequency reference
}

// IdentifierReport is the structure for C4 output, also used by snippet system.
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

// ToolSpec documents a tool's contract (name + schemas).
type ToolSpec struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}