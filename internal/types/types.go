package types

// Generic references / descriptors ------------------------------------------------

type DocRef struct {
    Path       string  `json:"path"`
    Reason     string  `json:"reason"`
    Confidence float64 `json:"confidence"`
}

type GlossEntry struct {
    Term       string  `json:"term"`
    Desc       string  `json:"desc"`
    Confidence float64 `json:"confidence"`
}

type KV struct {
    Key   string `json:"kind"`
    Value string `json:"desc"`
}

type NodeLite struct {
    ID         string   `json:"id"`
    Name       string   `json:"name"`
    Kind       string   `json:"kind"`
    Confidence float64  `json:"confidence"`
    Provenance []string `json:"provenance,omitempty"`
}

// Phase outputs ------------------------------------------------------------------

type P0Out struct {
    TopDocs     []DocRef     `json:"top_docs"`
    EntryPoints []DocRef     `json:"entry_points"`
    Glossary    []GlossEntry `json:"glossary_seed"`
    NextActions []string     `json:"next_actions"`
}

type P1Out struct {
    Taxonomy      []KV         `json:"taxonomy"`
    ParentNodes   []NodeLite   `json:"parent_nodes"`
    Glossary      []GlossEntry `json:"glossary"`
    ReadingPolicy []string     `json:"reading_policy"`
    ReadTargets   []DocRef     `json:"read_targets"`
}

type FieldWithConf struct {
    Summary    string   `json:"summary"`
    Confidence float64  `json:"confidence"`
    Provenance []string `json:"provenance,omitempty"`
}

// P2 -----------------------------------------------------------------------------

type APIDecl struct {
    Name       string   `json:"name"`
    Kind       string   `json:"kind"`
    Path       string   `json:"path"`
    Signature  string   `json:"signature,omitempty"`
    Provenance []string `json:"provenance,omitempty"`
}

type IdentDecl struct {
    Name       string   `json:"name"`
    Kind       string   `json:"kind"`
    Path       string   `json:"path"`
    Provenance []string `json:"provenance,omitempty"`
}

type P2Out struct {
    Dir          string         `json:"dir"`
    Role         FieldWithConf  `json:"role"`
    PublicAPI    []APIDecl      `json:"public_api"`
    Identifiers  []IdentDecl    `json:"identifiers"`
    NotableFiles []DocRef       `json:"notable_files"`
}

// P3 -----------------------------------------------------------------------------

type P3Out struct {
    Links []Edge `json:"links"`
    Notes []string `json:"notes"`
}

type Edge struct {
    From       string   `json:"from"`
    To         string   `json:"to"`
    Kind       string   `json:"kind"`
    Confidence float64  `json:"confidence"`
    Provenance []string `json:"provenance,omitempty"`
}
