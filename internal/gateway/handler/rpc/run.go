package rpc

import (
	"context"
	"fmt"
	"strings"

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
	out, err := h.svc.StartRun(ctx, req.Msg)
	if err != nil {
		return nil, toRunError(err)
	}
	return connect.NewResponse(out), nil
}

func toRunError(err error) error {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "required"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case strings.Contains(msg, "not found"):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return connect.NewError(connect.CodeInternal, fmt.Errorf("run service failed: %w", err))
	}
}
