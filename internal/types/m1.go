package types

type M1Hints struct {
    Targets []string `json:"targets,omitempty"`
    Notes   []string `json:"notes,omitempty"`
}

type M1Limits struct {
    MaxNext int `json:"max_next"`
}

type M1KeyComponent struct {
    Name          string        `json:"name"`
    Kind          string        `json:"kind"`
    Responsibility string       `json:"responsibility"`
    Evidence      []EvidenceRef `json:"evidence"`
}

type M1TechStack struct {
    Platforms  []string `json:"platforms"`
    Languages  []string `json:"languages"`
    BuildTools []string `json:"build_tools"`
}

type M1Hypothesis struct {
    // Purpose: one or two sentences that explain the purpose of the
    // system and its overall architecture, including notable external
    // nodes/services the system interacts with.
    Purpose        string           `json:"purpose"`
    Summary        string           `json:"summary"`
    KeyComponents  []M1KeyComponent `json:"key_components"`
    ExecutionModel string           `json:"execution_model"`
    TechStack      M1TechStack      `json:"tech_stack"`
    Assumptions    []string         `json:"assumptions"`
    Unknowns       []string         `json:"unknowns"`
    Confidence     float64          `json:"confidence"`
}

type M1NextFile struct {
    Path         string   `json:"path"`
    Reason       string   `json:"reason"`
    WhatToCheck  []string `json:"what_to_check"`
    Priority     int      `json:"priority"`
}

type M1NextPattern struct {
    Pattern      string   `json:"pattern"`
    Reason       string   `json:"reason"`
    WhatToCheck  []string `json:"what_to_check"`
    Priority     int      `json:"priority"`
}

type M1Contradiction struct {
    Claim     string        `json:"claim"`
    Supports  []EvidenceRef `json:"supports"`
    Conflicts []EvidenceRef `json:"conflicts"`
    Note      string        `json:"note"`
}

type M1Out struct {
    ArchitectureHypothesis M1Hypothesis       `json:"architecture_hypothesis"`
    NextFiles              []M1NextFile       `json:"next_files"`
    NextPatterns           []M1NextPattern    `json:"next_patterns"`
    Contradictions         []M1Contradiction  `json:"contradictions"`
    NeedsInput             []string           `json:"needs_input"`
    StopWhen               []string           `json:"stop_when"`
    Notes                  []string           `json:"notes"`
}

// M1In bundles inputs for the M1 milestone to align with M1's single-arg Run.
type M1In struct {
    FileIndex []FileIndexEntry `json:"file_index"`
    MDDocs    []MDDoc          `json:"md_docs"`
    Hints     *M1Hints         `json:"hints,omitempty"`
    Limits    *M1Limits        `json:"limits,omitempty"`
}
