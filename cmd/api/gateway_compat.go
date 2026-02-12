package main

import "insightify/internal/gateway/handler"

// RunContext is a type alias kept for backward compatibility.
type RunContext = handler.RunContext

var gatewayApp = handler.DefaultApp()

func NewRunContext(repoName string, projectID string) (*RunContext, error) {
	return handler.NewRunContext(repoName, projectID)
}
