package rpc

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	insightifyv1 "insightify/gen/go/insightify/v1"
	gatewayui "insightify/internal/gateway/service/ui"
)

type UiHandler struct {
	svc *gatewayui.Service
}

func NewUiHandler(svc *gatewayui.Service) *UiHandler {
	return &UiHandler{svc: svc}
}

func (h *UiHandler) GetDocument(ctx context.Context, req *connect.Request[insightifyv1.GetUiDocumentRequest]) (*connect.Response[insightifyv1.GetUiDocumentResponse], error) {
	out, err := h.svc.GetDocument(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UiHandler) GetProjectTabDocument(ctx context.Context, req *connect.Request[insightifyv1.GetProjectUiDocumentRequest]) (*connect.Response[insightifyv1.GetProjectUiDocumentResponse], error) {
	out, err := h.svc.GetProjectTabDocument(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UiHandler) ApplyOps(ctx context.Context, req *connect.Request[insightifyv1.ApplyUiOpsRequest]) (*connect.Response[insightifyv1.ApplyUiOpsResponse], error) {
	out, err := h.svc.ApplyOps(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}

func toUIError(err error) error {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "required") || strings.Contains(msg, "unsupported") {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("ui service failed: %w", err))
}
