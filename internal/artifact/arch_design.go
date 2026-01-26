package artifact

type ArchDesignHints struct {
	Targets []string `json:"targets,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

type ArchDesignKeyComponent struct {
	Name           string        `json:"name"`
	Kind           string        `json:"kind"`
	Responsibility string        `json:"responsibility"`
	Evidence       []EvidenceRef `json:"evidence"`
}

type ArchDesignTechStack struct {
	Platforms  []string `json:"platforms"`
	Languages  []string `json:"languages"`
	BuildTools []string `json:"build_tools"`
}

type ArchDesignHypothesis struct {
	// Purpose: one or two sentences that explain the purpose of the
	// system and its overall architecture, including notable external
	// nodes/services the system interacts with.
	Purpose        string           `json:"purpose"`
	Summary        string           `json:"summary"`
	KeyComponents  []ArchDesignKeyComponent `json:"key_components"`
	ExecutionModel string           `json:"execution_model"`
	TechStack      ArchDesignTechStack      `json:"tech_stack"`
	Assumptions    []string         `json:"assumptions"`
	Unknowns       []string         `json:"unknowns"`
	Confidence     float64          `json:"confidence"`
}

type ArchDesignContradiction struct {
	Claim     string        `json:"claim"`
	Supports  []EvidenceRef `json:"supports"`
	Conflicts []EvidenceRef `json:"conflicts"`
	Note      string        `json:"note"`
}

type ArchDesignOut struct {
	ArchitectureHypothesis ArchDesignHypothesis      `json:"architecture_hypothesis" prompt_type:"ArchitectureHypothesis" prompt_desc:"What the system does and how it is structured, including external nodes/services."`
	Contradictions         []ArchDesignContradiction `json:"contradictions" prompt_type:"[]Contradiction" prompt_desc:"Claims with supporting and conflicting evidence."`
}

// ArchDesignIn bundles inputs for the ArchDesign milestone to align with M1's single-arg Run.
type ArchDesignIn struct {
	Repo         string           `json:"repo"`
	LibraryRoots []string         `json:"library_roots"`
	FileIndex    []FileIndexEntry `json:"file_index"`
	MDDocs       []MDDoc          `json:"md_docs"`
	Hints        *ArchDesignHints         `json:"hints,omitempty"`
}
