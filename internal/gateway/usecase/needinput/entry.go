package needinput

import (
	"fmt"
	"strings"

	"connectrpc.com/connect"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

// NeedUserInputEntryDeps defines dependencies for SubmitRunInput handling.
type NeedUserInputEntryDeps struct {
	ActiveRunIDByProject func(projectID string) string
	Submit               func(projectID, runID, interactionID, input string) (string, error)
}

// NeedUserInputEntry handles request preprocessing and delegation for NeedUserInput.
type NeedUserInputEntry struct {
	deps NeedUserInputEntryDeps
}

func NewNeedUserInputEntry(deps NeedUserInputEntryDeps) *NeedUserInputEntry {
	return &NeedUserInputEntry{deps: deps}
}

func (e *NeedUserInputEntry) Handle(req *connect.Request[insightifyv1.SubmitRunInputRequest]) (*connect.Response[insightifyv1.SubmitRunInputResponse], error) {
	if req == nil || req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("request is required"))
	}
	in, err := PrepareSubmitRunInput(
		req.Msg.GetProjectId(),
		req.Msg.GetRunId(),
		req.Msg.GetInput(),
		e.deps.ActiveRunIDByProject,
	)
	if err != nil {
		return nil, err
	}
	if e.deps.Submit == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("submit dependency is not configured"))
	}
	if _, err := e.deps.Submit(in.ProjectID, in.RunID, "", in.Input); err != nil {
		return nil, err
	}
	return connect.NewResponse(&insightifyv1.SubmitRunInputResponse{RunId: in.RunID}), nil
}

// SendMessageEntryDeps defines dependencies for SendMessage handling.
type SendMessageEntryDeps struct {
	ConversationIDByRun func(runID string) string
	Submit              func(projectID, runID, interactionID, input string) (string, error)
}

// SendMessageEntry handles request preprocessing and delegation for SendMessage.
type SendMessageEntry struct {
	deps SendMessageEntryDeps
}

func NewSendMessageEntry(deps SendMessageEntryDeps) *SendMessageEntry {
	return &SendMessageEntry{deps: deps}
}

func (e *SendMessageEntry) Handle(req *connect.Request[insightifyv1.SendMessageRequest]) (*connect.Response[insightifyv1.SendMessageResponse], error) {
	if req == nil || req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("request is required"))
	}
	in, err := PrepareSendMessage(
		req.Msg.GetProjectId(),
		req.Msg.GetRunId(),
		req.Msg.GetInteractionId(),
		req.Msg.GetInput(),
	)
	if err != nil {
		return nil, err
	}
	conversationID := strings.TrimSpace(req.Msg.GetConversationId())
	if conversationID == "" && e.deps.ConversationIDByRun != nil {
		conversationID = strings.TrimSpace(e.deps.ConversationIDByRun(in.RunID))
	}
	if e.deps.Submit == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("submit dependency is not configured"))
	}
	gotInteractionID, err := e.deps.Submit(in.ProjectID, in.RunID, in.InteractionID, in.Input)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewResponse(&insightifyv1.SendMessageResponse{
		Accepted:       true,
		InteractionId:  gotInteractionID,
		ConversationId: conversationID,
	}), nil
}
