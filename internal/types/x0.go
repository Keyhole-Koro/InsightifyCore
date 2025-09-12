package types

type X0In struct {
	Repo      string     `json:"repo"`
	ExtCounts []ExtCount `json:"ext_counts"`
	Roots     M0Out      `json:"roots"`
}

type RuntimeConfig struct {
	Path    string `json:"path"`    // e.g. "config/tsconfig.json"
	Ext     string `json:"ext"`     // e.g. ".json"
	Content string `json:"content"` // raw text content (may be truncated)
}

type X0Out struct {
	Specs []ExtractorSpec `json:"specs"`
}

type ExtractorSpec struct {
	Ext            string         `json:"ext"`      // e.g. ".js"
	Language       string         `json:"language"` // e.g. "JavaScript"
	Rules          Rules          `json:"rules"`
	NormalizeHints NormalizeHints `json:"normalize_hints"`
}

// X0Spec is an alias for ExtractorSpec for compatibility with older code.
type X0Spec = ExtractorSpec

type Rules struct {
	Keywords  []string `json:"keywords"`   // tokens like "import","from","require"
	PathSplit []string `json:"path_split"` // separators to split around the path
}

type NormalizeHints struct {
	Alias []AliasPair `json:"alias"` // optional alias normalization map
}

type AliasPair struct {
	Original   string `json:"original"`
	Normalized string `json:"normalized"`
}
