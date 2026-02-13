package handler

import (
	"context"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	"insightify/internal/ui"

	"connectrpc.com/connect"
)

func (s *Service) SendMessage(_ context.Context, req *connect.Request[insightifyv1.SendMessageRequest]) (*connect.Response[insightifyv1.SendMessageResponse], error) {
	projectID, runID, interactionID, input, err := s.prepareSendMessage(req)
	if err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(req.Msg.GetConversationId())
	if conversationID == "" {
		conversationID = s.app.Interaction().ConversationIDByRun(runID)
	}
	gotInteractionID, err := s.app.Interaction().SubmitUserInput(projectID, runID, interactionID, input)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&insightifyv1.SendMessageResponse{
		Accepted:       true,
		InteractionId:  gotInteractionID,
		ConversationId: conversationID,
	}), nil
}

// ---------------------------------------------------------------------------
// chat helpers
// ---------------------------------------------------------------------------

func toProtoUINode(node ui.Node) *insightifyv1.UiNode {
	if strings.TrimSpace(node.ID) == "" {
		return nil
	}
	return ui.ToProtoNode(node)
}
