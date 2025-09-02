package types

// M0 summarizes the repository surface for a lightweight
// directory classification pass.
type M0In struct {
    ExtCounts  map[string]int `json:"ext_counts"`
    DirsDepth1 []string       `json:"dirs_depth1"` // e.g., ["src", "internal", "config"]
    DirsDepth2 []string       `json:"dirs_depth2"` // e.g., ["src/app", "src/lib", "internal/handlers"]
}

// M0 identifies likely roots for main code, libraries, and configs.
type M0Out struct {
    MainSourceRoots []string `json:"main_source_roots"`
    LibraryRoots    []string `json:"library_roots"`
    ConfigRoots     []string `json:"config_roots"`
    RuntimeConfigRoots []string `json:"runtime_config_roots,omitempty"`
    Notes           []string `json:"notes,omitempty"`
}
