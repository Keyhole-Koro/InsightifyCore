package artifact

// InitPurposeIn is the input for the interactive init_purpose worker.
type InitPurposeIn struct {
	UserInput   string `json:"user_input"`
	IsBootstrap bool   `json:"is_bootstrap"`
}

// InitPurposeOut is the structured output of init_purpose.
type InitPurposeOut struct {
	AssistantMessage string `json:"assistant_message"`
	Purpose          string `json:"purpose"`
	RepoURL          string `json:"repo_url"`
	NeedMoreInput    bool   `json:"need_more_input"`
	FollowupQuestion string `json:"followup_question"`
}
