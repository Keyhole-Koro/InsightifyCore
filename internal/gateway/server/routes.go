package server

import (
	"net/http"

	"insightify/gen/go/insightify/v1/insightifyv1connect"
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/handler/rpc"
	"insightify/internal/gateway/middleware"
)

func NewMux(
	projectHandler *rpc.ProjectHandler,
	runHandler *rpc.RunHandler,
	userInteractionHandler *rpc.UserInteractionHandler,
	uiHandler *rpc.UiHandler,
	traceHandler *handler.TraceHandler,
) http.Handler {
	mux := http.NewServeMux()

	// RPC Handlers
	mux.Handle(insightifyv1connect.NewProjectServiceHandler(projectHandler))
	mux.Handle(insightifyv1connect.NewRunServiceHandler(runHandler))
	mux.Handle(insightifyv1connect.NewUserInteractionServiceHandler(userInteractionHandler))
	mux.Handle(insightifyv1connect.NewUiServiceHandler(uiHandler))

	// Debug Handlers
	mux.HandleFunc("/debug/frontend-trace", traceHandler.HandleFrontendTrace)
	mux.HandleFunc("/debug/run-logs", traceHandler.HandleRunLogs)

	// Middleware
	return middleware.CORS(mux)
}
