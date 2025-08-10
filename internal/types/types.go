package types

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

// P1 ----------------------------------------------------------------

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

// P2 --------------------------------------------------------

type FieldWithConf struct {
    Summary    string   `json:"summary"`
    Confidence float64  `json:"confidence"`
    Provenance []string `json:"provenance,omitempty"`
}

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

// P3 --------------------------------------------------------

type ProvenanceRef struct {
    File  string `json:"file"`
    Lines [2]int `json:"lines"`
}

type Node struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`
    Kind        string          `json:"kind"`
    Layer       int             `json:"layer"`
    Origin      string          `json:"origin,omitempty"` // abstract|code
    Paths       []string        `json:"paths,omitempty"`
    Span        []ProvenanceRef `json:"span,omitempty"`
    Identifiers []string        `json:"identifiers,omitempty"`
    Confidence  float64         `json:"confidence"`
    Provenance  []ProvenanceRef `json:"provenance"`
}

type P3Out struct {
    Nodes         []Node  `json:"nodes"`
    OpenQuestions []string `json:"open_questions"`
}

// -------- P4 Evidence (language-agnostic) --------

// Signal is a language-agnostic hint extracted from code.
// Keep it abstract: no HTTP/SQL, no specific protocols, no language-specific terms.
type Signal struct {
	Kind  string            `json:"kind"`              // bind | invoke | io | declare | annotate | file_head
	File  string            `json:"file"`
	Range [2]int            `json:"range,omitempty"`   // [startLine, endLine]
	Attrs map[string]string `json:"attrs,omitempty"`   // callee/ref/channel_hint/verb_hint/key/topic/boundary_hint...
	Text  string            `json:"text,omitempty"`    // short head snippet (optional)
}

type P4Evidence struct {
	Dir     string   `json:"dir"`
	Signals []Signal `json:"signals"`
}

// -------- P4 Output --------

// Edge is abstract, technology-neutral.
type Edge struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Type       string            `json:"type"`                 // contains|depends_on|invokes|exchanges|persists|configures|observes
	Attrs      map[string]string `json:"attrs,omitempty"`      // channel=network|storage|message|process|unknown, sync=sync|async|unknown, boundary=internal|external|unknown, verb, etc.
	Evidence   []ProvenanceRef   `json:"evidence,omitempty"`
	Confidence float64           `json:"confidence"`
}

// Artifacts are also abstract. Keep interfaces/schemas/config only.
type InterfaceArtifact struct {
	Name       string          `json:"name"`
	Kind       string          `json:"kind"` // api|rpc|cli|event|other
	Where      string          `json:"where"`
	Provenance []ProvenanceRef `json:"provenance,omitempty"`
}
type SchemaItem struct {
	Name       string          `json:"name"`
	Where      string          `json:"where"`
	Provenance []ProvenanceRef `json:"provenance,omitempty"`
}
type ConfigItem struct {
	Key        string          `json:"key"`
	Where      string          `json:"where"`
	Provenance []ProvenanceRef `json:"provenance,omitempty"`
}

type Artifacts struct {
	Interfaces []InterfaceArtifact `json:"interfaces,omitempty"`
	Schemas    []SchemaItem        `json:"schemas,omitempty"`
	Config     []ConfigItem        `json:"config,omitempty"`
}

type P4Out struct {
	Edges     []Edge     `json:"edges"`
	Artifacts Artifacts  `json:"artifacts"`
}

// P5 -------------------------------------------------

// GraphState is the unified graph after P5 normalization.
type GraphState struct {
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	Artifacts Artifacts `json:"artifacts"`
}

// Change is an atomic normalization operation proposed by P5.
// Supported ops:
// - merge_nodes: merge multiple nodes into one (fields unioned; edges retargeted)
// - promote: change node kind/layer upward (e.g., module -> subsystem)
// - demote: change node kind/layer downward (e.g., subsystem -> module)
// - drop_node: remove a node and its incident edges
// - drop_edge: remove an edge by id
// - add_node: add a new node (allowed kinds: schema|config|interface)
type Change struct {
	Op      string   `json:"op"`                     // merge_nodes|promote|demote|drop_node|drop_edge|add_node
	From    []string `json:"from,omitempty"`         // merge_nodes
	To      string   `json:"to,omitempty"`           // merge_nodes target
	ID      string   `json:"id,omitempty"`           // node id (promote/demote/drop_node/add_node) or edge id (drop_edge)
	ToKind  string   `json:"to_kind,omitempty"`      // promote/demote
	Name    string   `json:"name,omitempty"`         // add_node
	Kind    string   `json:"kind,omitempty"`         // add_node
	Layer   int      `json:"layer,omitempty"`        // add_node
	Origin  string   `json:"origin,omitempty"`       // add_node (abstract|code)
	Paths   []string `json:"paths,omitempty"`        // add_node
	Reason  string   `json:"reason,omitempty"`
}

type P5Out struct {
	Changes    []Change   `json:"changes"`
	GraphState GraphState `json:"graph_state"`
}