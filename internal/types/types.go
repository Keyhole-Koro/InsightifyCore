package types

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
	Path     string `json:"path,omitempty"`     // optional: where this question points to
	Question string `json:"question"`           // natural language question
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
