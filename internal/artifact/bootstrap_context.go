package artifact

import "strings"

// BootstrapContext is persisted by the bootstrap worker and consumed by downstream workers.
type BootstrapContext struct {
	Purpose   string `json:"purpose,omitempty"`
	RepoURL   string `json:"repo_url,omitempty"`
	UserInput string `json:"user_input,omitempty"`
}

func (c BootstrapContext) Normalize() BootstrapContext {
	return BootstrapContext{
		Purpose:   strings.TrimSpace(c.Purpose),
		RepoURL:   strings.TrimSpace(c.RepoURL),
		UserInput: strings.TrimSpace(c.UserInput),
	}
}

func (c BootstrapContext) IsEmpty() bool {
	n := c.Normalize()
	return n.Purpose == "" && n.RepoURL == "" && n.UserInput == ""
}
