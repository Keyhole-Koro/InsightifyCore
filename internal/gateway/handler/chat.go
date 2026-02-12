package handler

import (
	"context"
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	chatuc "insightify/internal/gateway/usecase/chat"
	"insightify/internal/ui"

	"connectrpc.com/connect"
)

func (s *Service) WatchChat(ctx context.Context, req *connect.Request[insightifyv1.WatchChatRequest], stream *connect.ServerStream[insightifyv1.ChatEvent]) error {
	s.app.ProjectStore().EnsureLoaded()
	prepared, err := chatuc.PrepareWatch(
		req.Msg.GetProjectId(),
		req.Msg.GetRunId(),
		req.Msg.GetConversationId(),
		req.Msg.GetFromSeq(),
		chatuc.WatchDeps{
			ConversationIDByRun:   s.app.Interaction().ConversationIDByRun,
			RunIDByConversation:   s.app.Interaction().RunIDByConversation,
			EnsureConversation:    s.app.Interaction().EnsureConversation,
			ProjectIDByRun:        s.app.Interaction().ProjectIDByRun,
			SubscribeConversation: s.app.Interaction().SubscribeConversation,
		},
	)
	if err != nil {
		return err
	}
	if prepared.Cancel != nil {
		defer prepared.Cancel()
	}
	for _, ev := range prepared.Snapshot {
		ev = chatuc.FillEvent(ev, prepared.ProjectID, prepared.RunID)
		if err := stream.Send(ev); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send chat snapshot event: %w", err))
		}
	}
	if prepared.Sub == nil {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-prepared.Sub:
			if !ok {
				return nil
			}
			if ev == nil {
				continue
			}
			ev = chatuc.FillEvent(ev, prepared.ProjectID, prepared.RunID)
			if err := stream.Send(ev); err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send chat event: %w", err))
			}
		}
	}
}

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

func buildLlmChatNode(runID, workerKey, text string, seq int64, isResponding bool, sendLocked bool, sendLockHint string) *insightifyv1.UiNode {
	node, ok := ui.BuildLLMChatNode(runID, workerKey, text, seq, isResponding, sendLocked, sendLockHint)
	if !ok {
		return nil
	}
	return ui.ToProtoNode(node)
}

func toProtoUINode(node ui.Node) *insightifyv1.UiNode {
	if strings.TrimSpace(node.ID) == "" {
		return nil
	}
	return ui.ToProtoNode(node)
}

func (s *Service) publishRunEventToChat(projectID, runID string, ev *insightifyv1.WatchRunResponse) {
	runID = strings.TrimSpace(runID)
	if runID == "" || ev == nil {
		return
	}
	conversationID := s.app.Interaction().ConversationIDByRun(runID)
	s.app.Interaction().EnsureConversation(runID, conversationID)
	chat := chatuc.MapRunEventToChatEvent(projectID, runID, conversationID, ev, chatuc.EventMapperDeps{
		GetRunNode: s.app.GetRunNode,
		BuildNode:  buildLlmChatNode,
		GetPending: s.app.Interaction().GetPending,
	})
	if chat == nil {
		return
	}
	s.app.Interaction().AppendChatEvent(runID, conversationID, chat)
}
