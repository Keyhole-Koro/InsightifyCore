package artifact

type CodeSpecsIn struct {
	Repo      string     `json:"repo"`
	ExtCounts []ExtCount `json:"ext_counts"`
	Roots     CodeRootsOut      `json:"roots"`
}

type CodeSpecsOut struct {
	FamilyKeys map[string][]string      `json:"familyKeys"`
	Specs      map[string]ExtractorSpec `json:"specs"`
	Families   []FamilySpec             `json:"families"`
}

type ExtractorSpec struct {
	Exts                []string       `json:"exts"`     // e.g. [".ts",".js"]
	Language            []Language     `json:"language"` // e.g. ["TypeScript","JavaScript"]
	Rules               Rules          `json:"rules"`
	CommentLinePattern  []string       `json:"comment_line_pattern"`
	CommentBlockPattern []string       `json:"comment_block_pattern"`
	NormalizeHints      NormalizeHints `json:"normalize_hints"`
}

// CodeSpecsSpec is an alias for ExtractorSpec for compatibility with older code.
type CodeSpecsSpec = ExtractorSpec

type Language struct {
	Name string   `json:"name"` // e.g. "TypeScript"
	Exts []string `json:"exts"` // e.g. [".ts"]
}

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

type FamilySpec struct {
	Family string        `json:"family"`
	Key    string        `json:"key"`
	Spec   ExtractorSpec `json:"spec"`
}
