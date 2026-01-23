package main

import (
	"net/http"
	"sync"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/gen/go/insightify/v1/insightifyv1connect"
	pipelinev1connect "insightify/gen/go/pipeline/v1/pipelinev1connect"
)

// apiServer wires Connect handlers and HTTP helpers.
type apiServer struct {
	specMu sync.RWMutex
	specs  map[string]*insightifyv1.GraphSpec

	runMu sync.RWMutex
	runs  map[string]*insightifyv1.Run
}

func newAPIServer() *apiServer {
	return &apiServer{
		specs: make(map[string]*insightifyv1.GraphSpec),
		runs:  make(map[string]*insightifyv1.Run),
	}
}

func buildMux(s *apiServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(insightifyv1connect.NewPipelineServiceHandler(s))
	mux.Handle(pipelinev1connect.NewGatewayServiceHandler(s))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
