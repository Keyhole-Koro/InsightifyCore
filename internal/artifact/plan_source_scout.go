package artifact

// PlanSourceScoutIn is the input for repository scouting in the plan phase.
type PlanSourceScoutIn struct {
	UserInput   string `json:"user_input"`
	IsBootstrap bool   `json:"is_bootstrap"`
}

// PlanSourceScoutOut is the structured output of plan_source_scout.
type PlanSourceScoutOut struct {
	RecommendedRepoURL string `json:"recommended_repo_url"`
	Explanation        string `json:"explanation"`
}
