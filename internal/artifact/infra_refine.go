package artifact

// InfraRefineIn reuses the previous X0 snapshot and provides extra evidence files.
type InfraRefineIn struct {
	Repo     string       `json:"repo"`
	Previous InfraContextOut        `json:"previous"`
	Files    []OpenedFile `json:"files"`
	Notes    []string     `json:"notes,omitempty"`
}

// InfraRefineOut includes the updated external overview plus a delta summary.
type InfraRefineOut struct {
	ExternalOverview ExternalOverview `json:"external_overview"`
	Delta            InfraRefineDelta          `json:"delta"`
	NeedsInput       []string         `json:"needs_input"`
	StopWhen         []string         `json:"stop_when"`
	Notes            []string         `json:"notes"`
}

type InfraRefineDelta struct {
	Added    []string     `json:"added"`
	Removed  []string     `json:"removed"`
	Modified []InfraRefineDeltaMod `json:"modified"`
}

type InfraRefineDeltaMod struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}
