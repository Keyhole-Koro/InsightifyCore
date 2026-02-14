package rpc

import (
	"context"
	"fmt"
	"log/slog"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/service/run"

	"connectrpc.com/connect"
)

type RunHandler struct {
	svc *run.Service
}

func NewRunHandler(svc *run.Service) *RunHandler {
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

func (h *RunHandler) WaitForInput(ctx context.Context, req *connect.Request[insightifyv1.WaitForInputRequest]) (*connect.Response[insightifyv1.WaitForInputResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *RunHandler) SendUserMessage(ctx context.Context, req *connect.Request[insightifyv1.SendUserMessageRequest]) (*connect.Response[insightifyv1.SendUserMessageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *RunHandler) SendServerMessage(ctx context.Context, req *connect.Request[insightifyv1.SendServerMessageRequest]) (*connect.Response[insightifyv1.SendServerMessageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *RunHandler) CloseInteraction(ctx context.Context, req *connect.Request[insightifyv1.CloseInteractionRequest]) (*connect.Response[insightifyv1.CloseInteractionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}
