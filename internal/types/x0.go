package types

type X0In struct {
    ExtReport []ExtReportEntry `json:"ext_report"`
    ExistingSpecs any `json:"existing_specs,omitempty"`
}

// ExtReportEntry summarizes per-extension evidence to help generate extraction rules.
type ExtReportEntry struct {
    Ext          string   `json:"ext"`
    Count        int      `json:"count"`
    SamplePaths  []string `json:"sample_paths"`
    HeadSnippet  string   `json:"head_snippet"`
    RandomLines  []string `json:"random_lines"`
}

type X0Out struct {
	Version int      `json:"version"`
	Specs   []X0Spec `json:"specs"`
}

type X0Spec struct {
	Ext           string        `json:"ext"`
	Language      string        `json:"language"`
	CommentStyles *CommentStyle `json:"comment_styles,omitempty"`
	StringDelims  []string      `json:"string_delims,omitempty"`
	Rules         []X0Rule      `json:"rules"`
	Notes         []string      `json:"notes,omitempty"`
	Confidence    float64       `json:"confidence"`
}

type CommentStyle struct {
	Line  []string       `json:"line,omitempty"`
	Block []BlockComment `json:"block,omitempty"`
}
type BlockComment struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type X0Rule struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"`  // "bind" only
	Style    string         `json:"style"` // "regex" or "regex+ebnf"
	Pattern  string         `json:"pattern"`
	Captures map[string]int `json:"captures"`          // e.g. {"module":1}
	Attrs    map[string]string `json:"attrs,omitempty"` // e.g. {"surface":"module"}
	Tests    struct {
		Pos []string `json:"pos"`
		Neg []string `json:"neg"`
	} `json:"tests"`
}
