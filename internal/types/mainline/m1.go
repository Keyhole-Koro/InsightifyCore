package mainline

import t "insightify/internal/types"

type M1Hints struct {
	Targets []string `json:"targets,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

type M1Limits struct {
	MaxNext int `json:"max_next"`
}

type M1KeyComponent struct {
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	Responsibility string          `json:"responsibility"`
	Evidence       []t.EvidenceRef `json:"evidence"`
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
	Path        string   `json:"path"`
	Reason      string   `json:"reason"`
	WhatToCheck []string `json:"what_to_check"`
	Priority    int      `json:"priority"`
}

type M1NextPattern struct {
	Pattern     string   `json:"pattern"`
	Reason      string   `json:"reason"`
	WhatToCheck []string `json:"what_to_check"`
	Priority    int      `json:"priority"`
}

type M1Contradiction struct {
	Claim     string          `json:"claim"`
	Supports  []t.EvidenceRef `json:"supports"`
	Conflicts []t.EvidenceRef `json:"conflicts"`
	Note      string          `json:"note"`
}

type M1Out struct {
	ArchitectureHypothesis M1Hypothesis      `json:"architecture_hypothesis" prompt_type:"ArchitectureHypothesis" prompt_desc:"What the system does and how it is structured, including external nodes/services."`
	NextFiles              []M1NextFile      `json:"next_files" prompt_type:"[]NextFile" prompt_desc:"Specific files to open next."`
	NextPatterns           []M1NextPattern   `json:"next_patterns" prompt_type:"[]NextPattern" prompt_desc:"Search patterns to explore next."`
	Contradictions         []M1Contradiction `json:"contradictions" prompt_type:"[]Contradiction" prompt_desc:"Claims with supporting and conflicting evidence."`
	NeedsInput             []string          `json:"needs_input" prompt_desc:"Missing inputs or questions for the human."`
	StopWhen               []string          `json:"stop_when" prompt_desc:"Convergence criteria."`
	Notes                  []string          `json:"notes" prompt_desc:"Short notes or caveats."`
}

// M1In bundles inputs for the M1 milestone to align with M1's single-arg Run.
type M1In struct {
	Repo       string             `json:"repo"`
	IgnoreDirs []string           `json:"ignore_dirs,omitempty"`
	FileIndex  []t.FileIndexEntry `json:"file_index"`
	MDDocs     []t.MDDoc          `json:"md_docs"`
	Hints      *M1Hints           `json:"hints,omitempty"`
	Limits     *M1Limits          `json:"limits,omitempty"`
}
