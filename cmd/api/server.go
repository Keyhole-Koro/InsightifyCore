package main

import (
	"net/http"

	"insightify/gen/go/insightify/v1/insightifyv1connect"
)

// apiServer wires Connect handlers and HTTP helpers.
type apiServer struct{}

func newAPIServer() *apiServer {
	return &apiServer{}
}

func buildMux(s *apiServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(insightifyv1connect.NewPipelineServiceHandler(s))
	mux.Handle(insightifyv1connect.NewLlmChatServiceHandler(s))

	// SSE endpoint for watching runs
	mux.HandleFunc("/api/watch/", s.handleWatchSSE)

	return mux
}
