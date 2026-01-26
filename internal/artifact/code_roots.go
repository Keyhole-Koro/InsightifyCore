package artifact

// CodeRoots summarizes the repository surface for a lightweight
// directory classification pass.
type CodeRootsIn struct {
	Repo      string         `json:"repo"`
	ExtCounts map[string]int `json:"ext_counts"`
	Dirs      []string       `json:"dirs_depth1"` // repo-relative folders encountered during the scan
}

// CodeRoots identifies likely roots for main code, libraries, and configs.
type CodeRootsOut struct {
	MainSourceRoots    []string `json:"main_source_roots" prompt_desc:"Primary application code directories."`
	LibraryRoots       []string `json:"library_roots" prompt_desc:"Shared libs or vendored deps to skip in analysis."`
	ConfigRoots        []string `json:"config_roots" prompt_desc:"Configuration/infra/ops directories."`
	RuntimeConfigRoots []string `json:"runtime_config_roots,omitempty" prompt_desc:"Directories whose files affect runtime behavior."`
	// Optional: specific files that represent configuration and runtime-config
	ConfigFiles        []string        `json:"config_files,omitempty" prompt_desc:"Specific config file paths."`
	RuntimeConfigFiles []string        `json:"runtime_config_files,omitempty" prompt_desc:"Runtime-impacting file paths."`
	BuildRoots         []string        `json:"build_roots,omitempty" prompt_desc:"Build or packaging directories."`
	Notes              []string        `json:"notes,omitempty" prompt_desc:"Short rationale or uncertainty notes."`
	RuntimeConfigs     []RuntimeConfig `json:"runtime_configs,omitempty" prompt_desc:"Runtime config files with {path, ext}."`
}

type RuntimeConfig struct {
	Path    string `json:"path"`    // e.g. "config/tsconfig.json"
	Ext     string `json:"ext"`     // e.g. ".json"
	Content string `json:"content"` // raw text content (may be truncated)
}
