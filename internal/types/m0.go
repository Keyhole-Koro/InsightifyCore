package types

type M0Hints struct {
    Targets []string `json:"targets,omitempty"`
    Notes   []string `json:"notes,omitempty"`
}

type M0Limits struct {
    MaxNext int `json:"max_next"`
}

type M0KeyComponent struct {
    Name          string        `json:"name"`
    Kind          string        `json:"kind"`
    Responsibility string       `json:"responsibility"`
    Evidence      []EvidenceRef `json:"evidence"`
}

type M0TechStack struct {
    Platforms  []string `json:"platforms"`
    Languages  []string `json:"languages"`
    BuildTools []string `json:"build_tools"`
}

type M0Hypothesis struct {
    // Purpose: one or two sentences that explain the purpose of the
    // system and its overall architecture, including notable external
    // nodes/services the system interacts with.
    Purpose        string           `json:"purpose"`
    Summary        string           `json:"summary"`
    KeyComponents  []M0KeyComponent `json:"key_components"`
    ExecutionModel string           `json:"execution_model"`
    TechStack      M0TechStack      `json:"tech_stack"`
    Assumptions    []string         `json:"assumptions"`
    Unknowns       []string         `json:"unknowns"`
    Confidence     float64          `json:"confidence"`
}

type M0NextFile struct {
    Path         string   `json:"path"`
    Reason       string   `json:"reason"`
    WhatToCheck  []string `json:"what_to_check"`
    Priority     int      `json:"priority"`
}

type M0NextPattern struct {
    Pattern      string   `json:"pattern"`
    Reason       string   `json:"reason"`
    WhatToCheck  []string `json:"what_to_check"`
    Priority     int      `json:"priority"`
}

type M0Contradiction struct {
    Claim     string        `json:"claim"`
    Supports  []EvidenceRef `json:"supports"`
    Conflicts []EvidenceRef `json:"conflicts"`
    Note      string        `json:"note"`
}

type M0Out struct {
    ArchitectureHypothesis M0Hypothesis       `json:"architecture_hypothesis"`
    NextFiles              []M0NextFile       `json:"next_files"`
    NextPatterns           []M0NextPattern    `json:"next_patterns"`
    Contradictions         []M0Contradiction  `json:"contradictions"`
    NeedsInput             []string           `json:"needs_input"`
    StopWhen               []string           `json:"stop_when"`
    Notes                  []string           `json:"notes"`
}

// M0In bundles inputs for the M0 milestone to align with M1's single-arg Run.
type M0In struct {
    FileIndex []FileIndexEntry `json:"file_index"`
    MDDocs    []MDDoc          `json:"md_docs"`
    Hints     *M0Hints         `json:"hints,omitempty"`
    Limits    *M0Limits        `json:"limits,omitempty"`
}
