package rpc

import (
	"context"

	"connectrpc.com/connect"

	insightifyv1 "insightify/gen/go/insightify/v1"
	gatewayui "insightify/internal/gateway/service/ui"
)

type UiWorkspaceHandler struct {
	svc *gatewayui.Service
}

func NewUiWorkspaceHandler(svc *gatewayui.Service) *UiWorkspaceHandler {
	return &UiWorkspaceHandler{svc: svc}
}

func (h *UiWorkspaceHandler) GetWorkspace(ctx context.Context, req *connect.Request[insightifyv1.GetUiWorkspaceRequest]) (*connect.Response[insightifyv1.GetUiWorkspaceResponse], error) {
	out, err := h.svc.GetWorkspace(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UiWorkspaceHandler) ListTabs(ctx context.Context, req *connect.Request[insightifyv1.ListUiTabsRequest]) (*connect.Response[insightifyv1.ListUiTabsResponse], error) {
	out, err := h.svc.ListTabs(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UiWorkspaceHandler) CreateTab(ctx context.Context, req *connect.Request[insightifyv1.CreateUiTabRequest]) (*connect.Response[insightifyv1.CreateUiTabResponse], error) {
	out, err := h.svc.CreateTab(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}

func (h *UiWorkspaceHandler) SelectTab(ctx context.Context, req *connect.Request[insightifyv1.SelectUiTabRequest]) (*connect.Response[insightifyv1.SelectUiTabResponse], error) {
	out, err := h.svc.SelectTab(ctx, req.Msg)
	if err != nil {
		return nil, toUIError(err)
	}
	return connect.NewResponse(out), nil
}
