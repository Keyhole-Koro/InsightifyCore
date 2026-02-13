package main

import (
	"insightify/internal/gateway/projectstore"
	"insightify/internal/gateway/runtime"
	"path/filepath"
)

// RunContext is a type alias kept for backward compatibility.
type RunContext = runtime.RunContext

var defaultProjectStore = projectstore.NewFromEnv(filepath.Join("tmp", "project_states.json"))

func NewRunContext(repoName string, projectID string) (*RunContext, error) {
	return runtime.NewRunContext(repoName, projectID)
}
