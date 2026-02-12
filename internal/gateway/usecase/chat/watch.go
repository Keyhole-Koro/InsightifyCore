package chat

import (
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"

	"connectrpc.com/connect"
)

type WatchDeps struct {
	ConversationIDByRun   func(runID string) string
	RunIDByConversation   func(conversationID string) string
	EnsureConversation    func(runID, conversationID string) string
	ProjectIDByRun        func(runID string) string
	SubscribeConversation func(conversationID string, fromSeq int64) ([]*insightifyv1.ChatEvent, <-chan *insightifyv1.ChatEvent, func())
}

type WatchPrepared struct {
	ProjectID      string
	RunID          string
	ConversationID string
	Snapshot       []*insightifyv1.ChatEvent
	Sub            <-chan *insightifyv1.ChatEvent
	Cancel         func()
}

func PrepareWatch(projectID, runID, conversationID string, fromSeq int64, deps WatchDeps) (WatchPrepared, error) {
	runID = strings.TrimSpace(runID)
	conversationID = strings.TrimSpace(conversationID)
	if runID == "" && conversationID == "" {
		return WatchPrepared{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("run_id or conversation_id is required"))
	}
	if conversationID == "" && deps.ConversationIDByRun != nil {
		conversationID = strings.TrimSpace(deps.ConversationIDByRun(runID))
	}
	if runID == "" && deps.RunIDByConversation != nil {
		runID = strings.TrimSpace(deps.RunIDByConversation(conversationID))
	}
	if strings.TrimSpace(conversationID) == "" {
		return WatchPrepared{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("conversation_id could not be resolved"))
	}
	if strings.TrimSpace(runID) != "" && deps.EnsureConversation != nil {
		deps.EnsureConversation(runID, conversationID)
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" && runID != "" && deps.ProjectIDByRun != nil {
		projectID = strings.TrimSpace(deps.ProjectIDByRun(runID))
	}
	var (
		snapshot []*insightifyv1.ChatEvent
		sub      <-chan *insightifyv1.ChatEvent
		cancel   func()
	)
	if deps.SubscribeConversation != nil {
		snapshot, sub, cancel = deps.SubscribeConversation(conversationID, fromSeq)
	}
	return WatchPrepared{
		ProjectID:      projectID,
		RunID:          runID,
		ConversationID: conversationID,
		Snapshot:       snapshot,
		Sub:            sub,
		Cancel:         cancel,
	}, nil
}

func FillEvent(ev *insightifyv1.ChatEvent, projectID, runID string) *insightifyv1.ChatEvent {
	if ev == nil {
		return nil
	}
	if strings.TrimSpace(ev.GetProjectId()) == "" && strings.TrimSpace(projectID) != "" {
		ev.ProjectId = strings.TrimSpace(projectID)
	}
	if strings.TrimSpace(ev.GetRunId()) == "" && strings.TrimSpace(runID) != "" {
		ev.RunId = strings.TrimSpace(runID)
	}
	return ev
}
