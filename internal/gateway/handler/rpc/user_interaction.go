package rpc

import (
	"context"
	"fmt"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/gateway/service/run"

	"connectrpc.com/connect"
)

type UserInteractionHandler struct {
	svc *run.Service
}

func NewUserInteractionHandler(svc *run.Service) *UserInteractionHandler {
	return &UserInteractionHandler{svc: svc}
}

func (h *UserInteractionHandler) WaitForInput(ctx context.Context, req *connect.Request[insightifyv1.WaitForInputRequest]) (*connect.Response[insightifyv1.WaitForInputResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *UserInteractionHandler) SendUserMessage(ctx context.Context, req *connect.Request[insightifyv1.SendUserMessageRequest]) (*connect.Response[insightifyv1.SendUserMessageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *UserInteractionHandler) SendServerMessage(ctx context.Context, req *connect.Request[insightifyv1.SendServerMessageRequest]) (*connect.Response[insightifyv1.SendServerMessageResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}

func (h *UserInteractionHandler) CloseInteraction(ctx context.Context, req *connect.Request[insightifyv1.CloseInteractionRequest]) (*connect.Response[insightifyv1.CloseInteractionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("not implemented"))
}
