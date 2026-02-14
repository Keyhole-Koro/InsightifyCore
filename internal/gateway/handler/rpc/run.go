package rpc

import (
	"context"
	"fmt"
	"log/slog"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/service/worker"

	"connectrpc.com/connect"
)

type RunHandler struct {
	svc *worker.Service
}

func NewRunHandler(svc *worker.Service) *RunHandler {
	return &RunHandler{svc: svc}
}

func (h *RunHandler) StartRun(ctx context.Context, req *connect.Request[insightifyv1.StartRunRequest]) (*connect.Response[insightifyv1.StartRunResponse], error) {
	// Logic delegates to service, currently unimplemented in service.
	// For now, handling the proto wrapping here.
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *RunHandler) WatchRun(ctx context.Context, req *connect.Request[insightifyv1.WatchRunRequest], stream *connect.ServerStream[insightifyv1.WatchRunResponse]) error {
	runID := req.Msg.GetRunId()
	slog.Info("WatchRun", "runID", runID)

	// In a real implementation, we would subscribe to the service.
	// For now, logic is unimplemented.
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}
