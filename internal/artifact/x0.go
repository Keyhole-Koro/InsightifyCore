package artifact

// X0In aggregates repository metadata that Stage X0 uses to reason about external systems.
type X0In struct {
	Repo                string              `json:"repo"`
	Roots               M0Out               `json:"roots"`
	Architecture        M1Out               `json:"architecture"`
	ConfigSamples       []OpenedFile        `json:"config_samples"`
	IdentifierSummaries []IdentifierSummary `json:"identifier_summaries"`
	ConfidenceThreshold float64             `json:"confidence_threshold"`
}

// IdentifierSummary captures high-signal identifiers (from C4) that touch external deps.
type IdentifierSummary struct {
	Path     string                  `json:"path"`
	Name     string                  `json:"name"`
	Role     string                  `json:"role,omitempty"`
	Summary  string                  `json:"summary,omitempty"`
	Lines    [2]int                  `json:"lines"`
	Scope    IdentifierScope         `json:"scope"`
	Requires []IdentifierRequirement `json:"requires,omitempty"`
	Source   string                  `json:"source,omitempty"`
	Notes    []string                `json:"notes,omitempty"`
}

// X0Out summarizes external systems plus verification gaps.
type X0Out struct {
	ExternalOverview ExternalOverview `json:"external_overview"`
	EvidenceGaps     []EvidenceGap    `json:"evidence_gaps"`
	Notes            []string         `json:"notes,omitempty"`
}

// ExternalOverview consolidates infra/build/runtime context.
type ExternalOverview struct {
	Purpose             string              `json:"purpose"`
	ArchitectureSummary string              `json:"architecture_summary"`
	ExternalSystems     []ExternalSystem    `json:"external_systems"`
	InfraComponents     []InfraComponent    `json:"infra_components"`
	BuildAndDeploy      []BuildDeployItem   `json:"build_and_deploy"`
	RuntimeConfigs      []RuntimeConfigItem `json:"runtime_configs"`
	Confidence          float64             `json:"confidence"`
}

// ExternalSystem documents an outside integration point.
type ExternalSystem struct {
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	Interaction string        `json:"interaction"`
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
	Confidence  float64       `json:"confidence"`
}

// InfraComponent highlights IaC/build assets.
type InfraComponent struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Paths      []string      `json:"paths,omitempty"`
	Summary    string        `json:"summary"`
	Evidence   []EvidenceRef `json:"evidence,omitempty"`
	Confidence float64       `json:"confidence"`
}

// BuildDeployItem captures CI/CD or packaging flows.
type BuildDeployItem struct {
	Tool       string        `json:"tool"`
	Usage      string        `json:"usage"`
	Evidence   []EvidenceRef `json:"evidence,omitempty"`
	Confidence float64       `json:"confidence"`
}

// RuntimeConfigItem notes environment-affecting configs.
type RuntimeConfigItem struct {
	Path        string        `json:"path"`
	Description string        `json:"description"`
	Evidence    []EvidenceRef `json:"evidence,omitempty"`
	Confidence  float64       `json:"confidence"`
}

// EvidenceGap is a low-confidence area that needs inspection.
type EvidenceGap struct {
	Topic        string          `json:"topic"`
	Question     string          `json:"question"`
	CurrentGuess string          `json:"current_guess,omitempty"`
	Confidence   float64         `json:"confidence"`
	Impact       string          `json:"impact,omitempty"`
	Suggested    []LookupRequest `json:"suggested"`
}

// LookupRequest instructs which file/snippet to open next.
type LookupRequest struct {
	Kind       string `json:"kind"` // file|identifier|config|doc
	Path       string `json:"path"`
	Identifier string `json:"identifier,omitempty"` // identifier name when Kind == "identifier"
	Reason     string `json:"reason"`
	Notes      string `json:"notes,omitempty"`
}
