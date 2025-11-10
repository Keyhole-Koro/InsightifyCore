package mainline

// M0 summarizes the repository surface for a lightweight
// directory classification pass.
type M0In struct {
	ExtCounts  map[string]int `json:"ext_counts"`
	DirsDepth1 []string       `json:"dirs_depth1"` // repo-relative folders encountered during the scan
}

// M0 identifies likely roots for main code, libraries, and configs.
type M0Out struct {
	MainSourceRoots    []string `json:"main_source_roots"`
	LibraryRoots       []string `json:"library_roots"`
	ConfigRoots        []string `json:"config_roots"`
	RuntimeConfigRoots []string `json:"runtime_config_roots,omitempty"`
	// Optional: specific files that represent configuration and runtime-config
	ConfigFiles        []string        `json:"config_files,omitempty"`
	RuntimeConfigFiles []string        `json:"runtime_config_files,omitempty"`
	BuildRoots         []string        `json:"build_roots,omitempty"`
	Notes              []string        `json:"notes,omitempty"`
	RuntimeConfigs     []RuntimeConfig `json:"runtime_configs,omitempty"`
}

type RuntimeConfig struct {
	Path    string `json:"path"`    // e.g. "config/tsconfig.json"
	Ext     string `json:"ext"`     // e.g. ".json"
	Content string `json:"content"` // raw text content (may be truncated)
}
