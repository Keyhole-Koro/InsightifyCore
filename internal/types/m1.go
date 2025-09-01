
package types

// M1In is the input envelope for milestone M1.
type M1In struct {
    Previous      any              `json:"previous"`       // prior hypothesis snapshot (opaque)
    OpenedFiles   []OpenedFile     `json:"opened_files"`   // newly opened files (content included)
    Focus         []FocusQuestion  `json:"focus"`          // questions we want to answer
    FileIndex     []FileIndexEntry `json:"file_index"`     // searchable index (path/size/kind/etc.)
    MDDocs        []MDDoc          `json:"md_docs"`        // markdown docs (text only)
    LimitMaxNext  int              `json:"limit_max_next"` // cap for next_files+next_patterns
}

// -------------------- M1Out --------------------

type M1Out struct {
    UpdatedHypothesis UpdatedHypothesis `json:"updated_hypothesis"`
    QuestionStatus    []QuestionStatus  `json:"question_status"`
    Delta             Delta             `json:"delta"`
    Contradictions    []Contradiction   `json:"contradictions"`
    NextFiles         []NextFileItem    `json:"next_files"`
    NextPatterns      []NextPatternItem `json:"next_patterns"`
    NeedsInput        []string          `json:"needs_input"`
    StopWhen          []string          `json:"stop_when"`
    Notes             []string          `json:"notes"`
}

// UpdatedHypothesis is the refined hypothesis with verification targets.
type UpdatedHypothesis struct {
    // Purpose: concise description of the system's purpose and architecture,
    // including external nodes/services it depends on or integrates with.
    Purpose           string               `json:"purpose"`
    Summary             string               `json:"summary"`
    KeyComponents       []KeyComponent       `json:"key_components"`
    ExecutionModel      string               `json:"execution_model"`
    TechStack           M0TechStack          `json:"tech_stack"`
    Assumptions         []string             `json:"assumptions"`
    Unknowns            []string             `json:"unknowns"`
    Confidence          float64              `json:"confidence"`
    VerificationTargets []VerificationTarget `json:"verification_targets"`
}

// VerificationTarget points to where a claim can be directly verified.
type VerificationTarget struct {
	Kind        string   `json:"kind"`                   // file|pattern|dir
	Path        string   `json:"path"`                   // exact path, dir, or glob
	Reason      string   `json:"reason"`                 // why this target verifies the claim
	WhatToCheck []string `json:"what_to_check"`          // concrete checks (symbols, keys, etc.)
	Coverage    string   `json:"coverage,omitempty"`     // high|medium|low
	Priority    int      `json:"priority,omitempty"`     // 1 is highest
}

// KeyComponent lists a core element with its responsibility and evidence.
type KeyComponent struct {
    Name          string      `json:"name"`
    Kind          string      `json:"kind"`
    Responsibility string     `json:"responsibility"`
    Evidence      []Evidence  `json:"evidence"`
    // LikelyDependentPaths: repository-relative folders or files that are
    // likely to depend on (import/call/use) this component.
    LikelyDependentPaths []string `json:"likely_dependent_paths"`
}

// Evidence references a file location used to support a claim.
// Lines is 1-based inclusive; may be null when unknown.
type Evidence struct {
	Path  string    `json:"path"`
	Lines *LineSpan `json:"lines"` // nil if unknown
}

// LineSpan is [start,end], 1-based inclusive.
type LineSpan [2]int

// QuestionStatus records the disposition of a focus question.
type QuestionStatus struct {
	Path     string     `json:"path"`
	Question string     `json:"question"`
	Status   string     `json:"status"`  // confirmed|refuted|inconclusive
	Evidence []Evidence `json:"evidence"`
	Note     string     `json:"note"`
}

// Delta captures changes compared to the previous hypothesis.
// 'before'/'after' are strings; pipeline normalization coerces non-strings.
type Delta struct {
	Added    []string   `json:"added"`
	Removed  []string   `json:"removed"`
	Modified []DeltaMod `json:"modified"`
}

type DeltaMod struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type Contradiction struct {
	Claim          string     `json:"claim"`
	Supports       []Evidence `json:"supports"`
	Conflicts      []Evidence `json:"conflicts"`
	ResolutionHint string     `json:"resolution_hint"`
}

// NextFileItem is a concrete file to open next.
type NextFileItem struct {
	Path         string   `json:"path"`
	Reason       string   `json:"reason"`
	WhatToCheck  []string `json:"what_to_check"`
	Priority     int      `json:"priority"`
}

// NextPatternItem is a directory or glob pattern to scan next.
type NextPatternItem struct {
	Pattern      string   `json:"pattern"`
	Reason       string   `json:"reason"`
	WhatToCheck  []string `json:"what_to_check"`
	Priority     int      `json:"priority"`
}
