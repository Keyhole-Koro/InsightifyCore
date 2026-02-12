package chat

import (
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	llminteraction "insightify/internal/llmInteraction"
)

const (
	InputRequiredPrefix = "INPUT_REQUIRED:"
	NodeReadyPrefix     = "NODE_READY"
)

type EventMapperDeps struct {
	GetRunNode func(runID string) *insightifyv1.UiNode
	BuildNode  func(runID, workerKey, text string, seq int64, isResponding bool, sendLocked bool, sendLockHint string) *insightifyv1.UiNode
	GetPending func(runID string) (llminteraction.PendingView, bool)
}

func parseInputRequiredMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, InputRequiredPrefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, InputRequiredPrefix))
}

func isNodeReadyMessage(message string) bool {
	return strings.TrimSpace(message) == NodeReadyPrefix
}

func MapRunEventToChatEvent(projectID, runID, conversationID string, ev *insightifyv1.WatchRunResponse, deps EventMapperDeps) *insightifyv1.ChatEvent {
	if ev == nil {
		return nil
	}
	chat := &insightifyv1.ChatEvent{
		ProjectId:      projectID,
		RunId:          runID,
		ConversationId: conversationID,
	}
	switch ev.GetEventType() {
	case insightifyv1.WatchRunResponse_EVENT_TYPE_LOG:
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_ASSISTANT_CHUNK
		chat.Text = ev.GetMessage()
		if deps.GetRunNode != nil {
			chat.Node = deps.GetRunNode(runID)
		}
		if chat.Node == nil && deps.BuildNode != nil {
			chat.Node = deps.BuildNode(runID, chat.WorkerKey, chat.Text, 0, true, false, "")
		}
		return chat
	case insightifyv1.WatchRunResponse_EVENT_TYPE_PROGRESS:
		if isNodeReadyMessage(ev.GetMessage()) {
			chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_ASSISTANT_CHUNK
			if deps.GetRunNode != nil {
				chat.Node = deps.GetRunNode(runID)
			}
			return chat
		}
		interactionID := parseInputRequiredMessage(ev.GetMessage())
		if interactionID == "" {
			return nil
		}
		chat.EventType = insightifyv1.ChatEvent_EVENT_TYPE_NEED_INPUT
		chat.InteractionId = interactionID
		if deps.GetPending != nil {
			if pending, ok := deps.GetPending(runID); ok {
				chat.WorkerKey = pending.WorkerKey
				chat.Text = pending.Prompt
			}
		}
		if chat.Text == "" && ev.GetClientView() != nil {
			chat.Text = strings.TrimSpace(ev.GetClientView().GetLlmResponse())
		}
		if deps.GetRunNode != nil {
			chat.Node = deps.GetRunNode(runID)
		}
		if chat.Node == nil && deps.BuildNode != nil {
			chat.Node = deps.BuildNode(runID, chat.WorkerKey, chat.Text, 0, false, false, "")
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
		if deps.GetRunNode != nil {
			chat.Node = deps.GetRunNode(runID)
		}
		if chat.Node == nil && deps.BuildNode != nil {
			chat.Node = deps.BuildNode(runID, chat.WorkerKey, chat.Text, 0, false, true, chat.Text)
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
		if deps.GetRunNode != nil {
			chat.Node = deps.GetRunNode(runID)
		}
		if chat.Node == nil && deps.BuildNode != nil {
			chat.Node = deps.BuildNode(runID, chat.WorkerKey, chat.Text, 0, false, false, "")
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
