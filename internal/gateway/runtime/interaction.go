package runtime

import (
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	llminteraction "insightify/internal/llmInteraction"
)

func (a *App) MarkRunStarted(projectID, runID string) {
	if a == nil || a.interaction == nil {
		return
	}
	a.interaction.MarkRunStarted(projectID, runID)
}

func (a *App) MarkRunFinished(projectID, runID string) {
	if a == nil || a.interaction == nil {
		return
	}
	a.interaction.MarkRunFinished(projectID, runID)
}

func (a *App) ActiveRunID(projectID string) string {
	if a == nil || a.interaction == nil {
		return ""
	}
	return a.interaction.ActiveRunID(projectID)
}

func (a *App) ProjectIDByRun(runID string) string {
	if a == nil || a.interaction == nil {
		return ""
	}
	return a.interaction.ProjectIDByRun(runID)
}

func (a *App) EnsureConversation(runID, conversationID string) string {
	if a == nil || a.interaction == nil {
		return ""
	}
	return a.interaction.EnsureConversation(runID, conversationID)
}

func (a *App) ConversationIDByRun(runID string) string {
	if a == nil || a.interaction == nil {
		return ""
	}
	return a.interaction.ConversationIDByRun(runID)
}

func (a *App) RunIDByConversation(conversationID string) string {
	if a == nil || a.interaction == nil {
		return ""
	}
	return a.interaction.RunIDByConversation(conversationID)
}

func (a *App) AppendChatEvent(runID, conversationID string, ev *insightifyv1.ChatEvent) {
	if a == nil || a.interaction == nil {
		return
	}
	a.interaction.AppendChatEvent(runID, conversationID, ev)
}

func (a *App) SubscribeConversation(conversationID string, fromSeq int64) ([]*insightifyv1.ChatEvent, <-chan *insightifyv1.ChatEvent, func()) {
	if a == nil || a.interaction == nil {
		return nil, nil, nil
	}
	return a.interaction.SubscribeConversation(conversationID, fromSeq)
}

func (a *App) RegisterNeedInput(projectID, runID, workerKey, prompt string) (string, error) {
	if a == nil || a.interaction == nil {
		return "", nil
	}
	return a.interaction.RegisterNeedInput(projectID, runID, workerKey, prompt)
}

func (a *App) WaitUserInput(runID string, timeout time.Duration) (string, error) {
	if a == nil || a.interaction == nil {
		return "", nil
	}
	return a.interaction.WaitUserInput(runID, timeout)
}

func (a *App) SubmitUserInput(projectID, runID, interactionID, input string) (string, error) {
	if a == nil || a.interaction == nil {
		return "", nil
	}
	return a.interaction.SubmitUserInput(projectID, runID, interactionID, input)
}

func (a *App) ClearUserInput(runID string) {
	if a == nil || a.interaction == nil {
		return
	}
	a.interaction.Clear(runID)
}

func (a *App) GetPending(runID string) (llminteraction.PendingView, bool) {
	if a == nil || a.interaction == nil {
		return llminteraction.PendingView{}, false
	}
	return a.interaction.GetPending(runID)
}
