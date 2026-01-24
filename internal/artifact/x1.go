package artifact

// X1In reuses the previous X0 snapshot and provides extra evidence files.
type X1In struct {
	Repo     string       `json:"repo"`
	Previous X0Out        `json:"previous"`
	Files    []OpenedFile `json:"files"`
	Notes    []string     `json:"notes,omitempty"`
}

// X1Out includes the updated external overview plus a delta summary.
type X1Out struct {
	ExternalOverview ExternalOverview `json:"external_overview"`
	Delta            X1Delta          `json:"delta"`
	NeedsInput       []string         `json:"needs_input"`
	StopWhen         []string         `json:"stop_when"`
	Notes            []string         `json:"notes"`
}

type X1Delta struct {
	Added    []string     `json:"added"`
	Removed  []string     `json:"removed"`
	Modified []X1DeltaMod `json:"modified"`
}

type X1DeltaMod struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
