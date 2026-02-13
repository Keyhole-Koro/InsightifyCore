package main

import (
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/runtime"
)

// RunContext is a type alias kept for backward compatibility.
type RunContext = runtime.RunContext

var gatewayApp = handler.DefaultApp()

func NewRunContext(repoName string, projectID string) (*RunContext, error) {
	return runtime.NewRunContext(repoName, projectID)
}
