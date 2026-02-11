package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	llminteraction "insightify/internal/llmInteraction"
	"insightify/internal/ui"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
)

const inputRequiredPrefix = "INPUT_REQUIRED:"
const nodeReadyPrefix = "NODE_READY"

var llmInteractionHandler llminteraction.Service = llminteraction.NewHandler()
var runNodeStore = struct {
	sync.RWMutex
	nodes map[string]*insightifyv1.UiNode
}{
	nodes: make(map[string]*insightifyv1.UiNode),
}

func ensureConversation(runID, conversationID string) string {
	return llmInteractionHandler.EnsureConversation(runID, conversationID)
}

func conversationIDByRun(runID string) string {
	return llmInteractionHandler.ConversationIDByRun(runID)
}

func runIDByConversation(conversationID string) string {
	return llmInteractionHandler.RunIDByConversation(conversationID)
}

func appendChatEvent(runID, conversationID string, ev *insightifyv1.ChatEvent) {
	llmInteractionHandler.AppendChatEvent(runID, conversationID, ev)
}

func subscribeConversation(conversationID string, fromSeq int64) ([]*insightifyv1.ChatEvent, <-chan *insightifyv1.ChatEvent, func()) {
	return llmInteractionHandler.SubscribeConversation(conversationID, fromSeq)
}

func registerPendingUserInput(sessionID, runID, workerKey, prompt string) (string, error) {
	return llmInteractionHandler.RegisterNeedInput(sessionID, runID, workerKey, prompt)
}

func waitPendingUserInput(runID string, timeout time.Duration) (string, error) {
	return llmInteractionHandler.WaitUserInput(runID, timeout)
}

func submitPendingUserInput(sessionID, runID, interactionID, input string) (string, error) {
	return llmInteractionHandler.SubmitUserInput(sessionID, runID, interactionID, input)
}

func clearPendingUserInput(runID string) {
	llmInteractionHandler.Clear(runID)
}

func setRunNode(runID string, node *insightifyv1.UiNode) {
	runID = strings.TrimSpace(runID)
	if runID == "" || node == nil {
		return
	}
	runNodeStore.Lock()
	runNodeStore.nodes[runID] = node
	runNodeStore.Unlock()
}

func getRunNode(runID string) *insightifyv1.UiNode {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	runNodeStore.RLock()
	node := runNodeStore.nodes[runID]
	runNodeStore.RUnlock()
	if node == nil {
		return nil
	}
	cloned, ok := proto.Clone(node).(*insightifyv1.UiNode)
	if !ok {
		return nil
	}
	return cloned
}

func clearRunNode(runID string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	runNodeStore.Lock()
	delete(runNodeStore.nodes, runID)
	runNodeStore.Unlock()
}

func getPendingUserInput(runID string) (llminteraction.PendingView, bool) {
	return llmInteractionHandler.GetPending(runID)
}

func parseInputRequiredMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, inputRequiredPrefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, inputRequiredPrefix))
}

func isNodeReadyMessage(message string) bool {
	return strings.TrimSpace(message) == nodeReadyPrefix
}

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

func mapRunEventToChatEvent(sessionID, runID, conversationID string, ev *insightifyv1.WatchRunResponse) *insightifyv1.ChatEvent {
	if ev == nil {
		return nil
	}
	chat := &insightifyv1.ChatEvent{
		SessionId:      sessionID,
		RunId:          runID,
		ConversationId: conversationID,
	}
	switch ev.GetEventType() {
	case insightifyv1.WatchRunResponse_EVENT_TYPE_LOG:
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_ASSISTANT_CHUNK
		chat.Text = ev.GetMessage()
		chat.Node = getRunNode(runID)
		if chat.Node == nil {
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, 0, true, false, "")
		}
		return chat
	case insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS:
		if isNodeReadyMessage(ev.GetMessage()) {
			chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_ASSISTANT_CHUNK
			chat.Node = getRunNode(runID)
			return chat
		}
		interactionID := parseInputRequiredMessage(ev.GetMessage())
		if interactionID == "" {
			return nil
		}
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_NEED_INPUT
		chat.InteractionId = interactionID
		if pending, ok := getPendingUserInput(runID); ok {
			chat.WorkerKey = pending.WorkerKey
			chat.Text = pending.Prompt
			if chat.SessionId == "" {
				chat.SessionId = pending.SessionID
			}
		}
		if chat.Text == "" && ev.GetClientView() != nil {
			chat.Text = strings.TrimSpace(ev.GetClientView().GetLlmResponse())
		}
		chat.Node = getRunNode(runID)
		if chat.Node == nil {
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, 0, false, false, "")
		}
		if state := chat.Node.GetLlmChat(); state != nil {
			state.SendLocked = true
			state.SendLockHint = chat.Text
			state.IsResponding = false
		}
		return chat
	case insightifyv1.WatchRunResponse_EVENT_TYPE_ERROR:
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_ERROR
		chat.Text = ev.GetMessage()
		chat.Node = getRunNode(runID)
		if chat.Node == nil {
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, 0, false, true, chat.Text)
		}
		if state := chat.Node.GetLlmChat(); state != nil {
			state.IsResponding = false
			state.SendLocked = true
			state.SendLockHint = chat.Text
		}
		return chat
	case insightifyv1.WatchRunResponse_EVENT_TYPE_COMPLETE:
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_COMPLETE
		if ev.GetClientView() != nil {
			chat.Text = strings.TrimSpace(ev.GetClientView().GetLlmResponse())
		}
		if chat.Text == "" {
			chat.Text = ev.GetMessage()
		}
		chat.Node = getRunNode(runID)
		if chat.Node == nil {
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, 0, false, false, "")
		}
		if state := chat.Node.GetLlmChat(); state != nil {
			state.IsResponding = false
			state.SendLocked = false
			state.SendLockHint = ""
		}
		return chat
	default:
		return nil
	}
}

func publishRunEventToChat(sessionID, runID string, ev *insightifyv1.WatchRunResponse) {
	runID = strings.TrimSpace(runID)
	if runID == "" || ev == nil {
		return
	}
	conversationID := conversationIDByRun(runID)
	ensureConversation(runID, conversationID)
	chat := mapRunEventToChatEvent(sessionID, runID, conversationID, ev)
	if chat == nil {
		return
	}
	appendChatEvent(runID, conversationID, chat)
}

func (s *apiServer) WatchChat(ctx context.Context, req *connect.Request[insightifyv1.WatchChatRequest], stream *connect.ServerStream[insightifyv1.ChatEvent]) error {
	ensureSessionStoreLoaded()

	runID := strings.TrimSpace(req.Msg.GetRunId())
	conversationID := strings.TrimSpace(req.Msg.GetConversationId())
	fromSeq := req.Msg.GetFromSeq()
	if runID == "" && conversationID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("run_id or conversation_id is required"))
	}
	if conversationID == "" {
		conversationID = conversationIDByRun(runID)
	}
	if runID == "" {
		runID = runIDByConversation(conversationID)
	}
	if strings.TrimSpace(conversationID) == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conversation_id could not be resolved"))
	}
	if strings.TrimSpace(runID) != "" {
		ensureConversation(runID, conversationID)
	}

	// Resolve session once at the start instead of per-event.
	sessionID := strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" && runID != "" {
		initRunStore.RLock()
		for sid, sess := range initRunStore.sessions {
			if strings.TrimSpace(sess.ActiveRunID) == runID {
				sessionID = sid
				break
			}
		}
		initRunStore.RUnlock()
	}

	snapshot, sub, cancel := subscribeConversation(conversationID, fromSeq)
	if cancel != nil {
		defer cancel()
	}

	for _, ev := range snapshot {
		if strings.TrimSpace(ev.GetSessionId()) == "" && sessionID != "" {
			ev.SessionId = sessionID
		}
		if strings.TrimSpace(ev.GetRunId()) == "" {
			ev.RunId = runID
		}
		if err := stream.Send(ev); err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send chat snapshot event: %w", err))
		}
	}

	if sub == nil {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-sub:
			if !ok {
				return nil
			}
			if ev == nil {
				continue
			}
			if strings.TrimSpace(ev.GetSessionId()) == "" && sessionID != "" {
				ev.SessionId = sessionID
			}
			if strings.TrimSpace(ev.GetRunId()) == "" {
				ev.RunId = runID
			}
			if err := stream.Send(ev); err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send chat event: %w", err))
			}
		}
	}
}

func (s *apiServer) SendMessage(_ context.Context, req *connect.Request[insightifyv1.SendMessageRequest]) (*connect.Response[insightifyv1.SendMessageResponse], error) {
	sessionID, runID, interactionID, input, err := prepareSendMessage(req)
	if err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(req.Msg.GetConversationId())
	if conversationID == "" {
		conversationID = conversationIDByRun(runID)
	}
	gotInteractionID, err := submitPendingUserInput(sessionID, runID, interactionID, input)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&insightifyv1.SendMessageResponse{
		Accepted:       true,
		InteractionId:  gotInteractionID,
		ConversationId: conversationID,
	}), nil
}
