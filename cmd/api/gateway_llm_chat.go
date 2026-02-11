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

var llmInteractionHandler llminteraction.Service = llminteraction.NewHandler()
var runNodeStore = struct {
	sync.RWMutex
	nodes map[string]*insightifyv1.UiNode
}{
	nodes: make(map[string]*insightifyv1.UiNode),
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

func mapRunEventToChatEvent(sessionID, runID string, seq int64, ev *insightifyv1.WatchRunResponse) *insightifyv1.ChatEvent {
	if ev == nil {
		return nil
	}
	chat := &insightifyv1.ChatEvent{
		SessionId: sessionID,
		RunId:     runID,
		Seq:       seq,
	}
	switch ev.GetEventType() {
	case insightifyv1.WatchRunResponse_EVENT_TYPE_LOG:
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_ASSISTANT_CHUNK
		chat.Text = ev.GetMessage()
		chat.Node = getRunNode(runID)
		if chat.Node == nil {
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, seq, true, false, "")
		}
		return chat
	case insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS:
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
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, seq, false, false, "")
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
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, seq, false, true, chat.Text)
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
			chat.Node = buildLlmChatNode(runID, chat.WorkerKey, chat.Text, seq, false, false, "")
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

func (s *apiServer) WatchChat(ctx context.Context, req *connect.Request[insightifyv1.WatchChatRequest], stream *connect.ServerStream[insightifyv1.ChatEvent]) error {
	ensureSessionStoreLoaded()
	runID := strings.TrimSpace(req.Msg.GetRunId())
	if runID == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("run_id is required"))
	}

	runStore.RLock()
	eventCh, ok := runStore.runs[runID]
	runStore.RUnlock()
	if !ok {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("run %s not found", runID))
	}

	// Resolve session once at the start instead of per-event.
	sessionID := strings.TrimSpace(req.Msg.GetSessionId())
	if sessionID == "" {
		initRunStore.RLock()
		for sid, sess := range initRunStore.sessions {
			if strings.TrimSpace(sess.ActiveRunID) == runID {
				sessionID = sid
				break
			}
		}
		initRunStore.RUnlock()
	}

	var seq int64
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-eventCh:
			if !ok {
				return nil
			}
			seq++
			chat := mapRunEventToChatEvent(sessionID, runID, seq, ev)
			if chat == nil {
				continue
			}
			if err := stream.Send(chat); err != nil {
				return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send chat event: %w", err))
			}
			if chat.EventType == insightifyv1.ChatEvent_EVENT_TYPE_COMPLETE || chat.EventType == insightifyv1.ChatEvent_EVENT_TYPE_ERROR {
				return nil
			}
		}
	}
}

func (s *apiServer) SendMessage(_ context.Context, req *connect.Request[insightifyv1.SendMessageRequest]) (*connect.Response[insightifyv1.SendMessageResponse], error) {
	sessionID, runID, interactionID, input, err := prepareSendMessage(req)
	if err != nil {
		return nil, err
	}
	gotInteractionID, err := submitPendingUserInput(sessionID, runID, interactionID, input)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&insightifyv1.SendMessageResponse{
		Accepted:      true,
		InteractionId: gotInteractionID,
	}), nil
}
