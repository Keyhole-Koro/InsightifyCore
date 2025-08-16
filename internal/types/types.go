package types

// Generic evidence reference with optional line range.
type EvidenceRef struct {
	Path  string  `json:"path"`
	Lines *[2]int `json:"lines"` // nil means unknown/unspecified lines
}

// -------- P0 --------

type FileIndexEntry struct {
	Path string `json:"path"`
	Ext  string `json:"ext"`
	Size int64  `json:"size"`
	LOC  int    `json:"loc"`
}

type MDDoc struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type P0Hints struct {
	Targets []string `json:"targets,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

type P0Limits struct {
	MaxNext int `json:"max_next"`
}

type P0KeyComponent struct {
	Name          string       `json:"name"`
	Kind          string       `json:"kind"`
	Responsibility string      `json:"responsibility"`
	Evidence      []EvidenceRef `json:"evidence"`
}

type P0TechStack struct {
	Platforms  []string `json:"platforms"`
	Languages  []string `json:"languages"`
	BuildTools []string `json:"build_tools"`
}

type P0Hypothesis struct {
	Summary       string          `json:"summary"`
	KeyComponents []P0KeyComponent `json:"key_components"`
	ExecutionModel string         `json:"execution_model"`
	TechStack     P0TechStack     `json:"tech_stack"`
	Assumptions   []string        `json:"assumptions"`
	Unknowns      []string        `json:"unknowns"`
	Confidence    float64         `json:"confidence"`
}

type P0NextFile struct {
	Path         string   `json:"path"`
	Reason       string   `json:"reason"`
	WhatToCheck  []string `json:"what_to_check"`
	Priority     int      `json:"priority"`
}

type P0NextPattern struct {
	Pattern      string   `json:"pattern"`
	Reason       string   `json:"reason"`
	WhatToCheck  []string `json:"what_to_check"`
	Priority     int      `json:"priority"`
}

type P0Contradiction struct {
	Claim     string        `json:"claim"`
	Supports  []EvidenceRef `json:"supports"`
	Conflicts []EvidenceRef `json:"conflicts"`
	Note      string        `json:"note"`
}

type P0Out struct {
	ArchitectureHypothesis P0Hypothesis      `json:"architecture_hypothesis"`
	NextFiles              []P0NextFile      `json:"next_files"`
	NextPatterns           []P0NextPattern   `json:"next_patterns"`
	Contradictions         []P0Contradiction `json:"contradictions"`
	NeedsInput             []string          `json:"needs_input"`
	StopWhen               []string          `json:"stop_when"`
	Notes                  []string          `json:"notes"`
}

// -------- P1 --------

type OpenedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FocusItem struct {
	Path        string   `json:"path"`
	WhatToCheck []string `json:"what_to_check"`
}

type P1Previous struct {
	ArchitectureHypothesis P0Hypothesis    `json:"architecture_hypothesis"`
	NextFiles              []P0NextFile    `json:"next_files"`
	NextPatterns           []P0NextPattern `json:"next_patterns"`
}

type P1KeyComponent struct {
	Name          string       `json:"name"`
	Kind          string       `json:"kind"`
	Responsibility string      `json:"responsibility"`
	Evidence      []EvidenceRef `json:"evidence"`
}

type P1TechStack struct {
	Platforms  []string `json:"platforms"`
	Languages  []string `json:"languages"`
	BuildTools []string `json:"build_tools"`
}

type P1Hypothesis struct {
	Summary       string           `json:"summary"`
	KeyComponents []P1KeyComponent `json:"key_components"`
	ExecutionModel string          `json:"execution_model"`
	TechStack     P1TechStack      `json:"tech_stack"`
	Assumptions   []string         `json:"assumptions"`
	Unknowns      []string         `json:"unknowns"`
	Confidence    float64          `json:"confidence"`
}

type P1QuestionStatus struct {
	Path     string        `json:"path"`
	Question string        `json:"question"`
	Status   string        `json:"status"` // "confirmed|refuted|inconclusive"
	Evidence []EvidenceRef `json:"evidence"`
	Note     string        `json:"note"`
}

type P1DeltaModified struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type P1Delta struct {
	Added    []string         `json:"added"`
	Removed  []string         `json:"removed"`
	Modified []P1DeltaModified `json:"modified"`
}

type P1Out struct {
	UpdatedHypothesis P1Hypothesis      `json:"updated_hypothesis"`
	QuestionStatus    []P1QuestionStatus `json:"question_status"`
	Delta             P1Delta            `json:"delta"`
	Contradictions    []P0Contradiction  `json:"contradictions"`
	NextFiles         []P0NextFile       `json:"next_files"`
	NextPatterns      []P0NextPattern    `json:"next_patterns"`
	NeedsInput        []string           `json:"needs_input"`
	StopWhen          []string           `json:"stop_when"`
	Notes             []string           `json:"notes"`
}

// Convenience input bundler for P1.
type P1In struct {
	Previous      P1Previous     `json:"previous"`
	OpenedFiles   []OpenedFile   `json:"opened_files"`
	Focus         []FocusItem    `json:"focus"`
	FileIndex     []FileIndexEntry `json:"file_index,omitempty"`
	MDDocs        []MDDoc        `json:"md_docs,omitempty"`
	LimitMaxNext  int            `json:"limit_max_next"`
}
