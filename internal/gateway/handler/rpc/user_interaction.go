package rpc

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	userinteraction "insightify/internal/gateway/service/user_interaction"

	"connectrpc.com/connect"
)

type UserInteractionHandler struct {
	svc *userinteraction.Service
}

func NewUserInteractionHandler(svc *userinteraction.Service) *UserInteractionHandler {
	return &UserInteractionHandler{svc: svc}
}

func (h *UserInteractionHandler) Wait(ctx context.Context, req *connect.Request[insightifyv1.WaitRequest]) (*connect.Response[insightifyv1.WaitResponse], error) {
	out, err := h.svc.Wait(ctx, req.Msg)
	if err != nil {
		return nil, toInteractionError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UserInteractionHandler) Send(ctx context.Context, req *connect.Request[insightifyv1.SendRequest]) (*connect.Response[insightifyv1.SendResponse], error) {
	out, err := h.svc.Send(ctx, req.Msg)
	if err != nil {
		return nil, toInteractionError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UserInteractionHandler) Close(ctx context.Context, req *connect.Request[insightifyv1.CloseRequest]) (*connect.Response[insightifyv1.CloseResponse], error) {
	out, err := h.svc.Close(ctx, req.Msg)
	if err != nil {
		return nil, toInteractionError(err)
	}
	return connect.NewResponse(out), nil
}

func toInteractionError(err error) error {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "required") {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("interaction service failed: %w", err))
}
