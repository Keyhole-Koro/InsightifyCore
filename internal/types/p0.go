package types

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
    // Purpose: one or two sentences that explain the purpose of the
    // system and its overall architecture, including notable external
    // nodes/services the system interacts with.
    Purpose       string          `json:"purpose"`
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

// P0In bundles inputs for the P0 phase to align with P1's single-arg Run.
type P0In struct {
    FileIndex []FileIndexEntry `json:"file_index"`
    MDDocs    []MDDoc          `json:"md_docs"`
    Hints     *P0Hints         `json:"hints,omitempty"`
    Limits    *P0Limits        `json:"limits,omitempty"`
}
