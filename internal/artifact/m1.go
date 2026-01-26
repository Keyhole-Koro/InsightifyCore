package artifact

type M1Hints struct {
	Targets []string `json:"targets,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

type M1KeyComponent struct {
	Name           string        `json:"name"`
	Kind           string        `json:"kind"`
	Responsibility string        `json:"responsibility"`
	Evidence       []EvidenceRef `json:"evidence"`
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

type M1Contradiction struct {
	Claim     string        `json:"claim"`
	Supports  []EvidenceRef `json:"supports"`
	Conflicts []EvidenceRef `json:"conflicts"`
	Note      string        `json:"note"`
}

type M1Out struct {
	ArchitectureHypothesis M1Hypothesis      `json:"architecture_hypothesis" prompt_type:"ArchitectureHypothesis" prompt_desc:"What the system does and how it is structured, including external nodes/services."`
	Contradictions         []M1Contradiction `json:"contradictions" prompt_type:"[]Contradiction" prompt_desc:"Claims with supporting and conflicting evidence."`
}

// M1In bundles inputs for the M1 milestone to align with M1's single-arg Run.
type M1In struct {
	Repo         string           `json:"repo"`
	LibraryRoots []string         `json:"library_roots"`
	FileIndex    []FileIndexEntry `json:"file_index"`
	MDDocs       []MDDoc          `json:"md_docs"`
	Hints        *M1Hints         `json:"hints,omitempty"`
}
