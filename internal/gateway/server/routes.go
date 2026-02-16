package server

import (
	"net/http"

	"insightify/gen/go/insightify/v1/insightifyv1connect"
	"insightify/internal/gateway/handler"
	"insightify/internal/gateway/handler/rpc"
	"insightify/internal/gateway/handler/ws"
	"insightify/internal/gateway/middleware"
)

func NewMux(
	projectHandler *rpc.ProjectHandler,
	runHandler *rpc.RunHandler,
	userInteractionHandler *ws.UserInteractionHandler,
	uiHandler *rpc.UiHandler,
	uiWorkspaceHandler *rpc.UiWorkspaceHandler,
	traceHandler *handler.TraceHandler,
) http.Handler {
	mux := http.NewServeMux()

	// RPC Handlers
	mux.Handle(insightifyv1connect.NewProjectServiceHandler(projectHandler))
	mux.Handle(insightifyv1connect.NewRunServiceHandler(runHandler))
	mux.Handle(insightifyv1connect.NewUiServiceHandler(uiHandler))
	mux.Handle(insightifyv1connect.NewUiWorkspaceServiceHandler(uiWorkspaceHandler))

	// Debug Handlers
	mux.HandleFunc("/ws/interaction", userInteractionHandler.HandleInteractionWS)
	mux.HandleFunc("/debug/frontend-trace", traceHandler.HandleFrontendTrace)
	mux.HandleFunc("/debug/run-logs", traceHandler.HandleRunLogs)

	// Middleware
	return middleware.CORS(mux)
}
