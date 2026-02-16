package userinteraction

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

// PublishOutput enqueues a server assistant message for the run.
func (s *Service) PublishOutput(ctx context.Context, runID, interactionID, message string) error {
	runID = strings.TrimSpace(runID)
	interactionID = strings.TrimSpace(interactionID)
	message = strings.TrimSpace(message)
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if message == "" {
		return fmt.Errorf("message is required")
	}

	var snapshot []byte
	s.mu.Lock()

	st := s.getOrCreateLocked(runID)
	if interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.outputQueue = append(st.outputQueue, outputMessage{
		interactionID: st.interactionID,
		message:       message,
	})
	st.conversation = append(st.conversation, conversationMessage{
		Seq:             len(st.conversation) + 1,
		Role:            "assistant",
		Content:         message,
		InteractionID:   st.interactionID,
		CreatedAtUnixMs: time.Now().UnixMilli(),
	})
	st.updatedAt = time.Now()
	snapshot = s.buildConversationSnapshotLocked(runID, st)
	notifyLocked(st)
	s.mu.Unlock()

	s.persistConversation(ctx, runID, snapshot)
	return nil
}

func (s *Service) Send(ctx context.Context, req *insightifyv1.SendRequest) (*insightifyv1.SendResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	input := strings.TrimSpace(req.GetInput())
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if input == "" {
		return nil, fmt.Errorf("input is required")
	}

	var snapshot []byte
	s.mu.Lock()

	st := s.getOrCreateLocked(runID)
	if st.closed {
		s.mu.Unlock()
		return &insightifyv1.SendResponse{
			Accepted:      false,
			InteractionId: st.interactionID,
		}, nil
	}
	if interactionID := strings.TrimSpace(req.GetInteractionId()); interactionID != "" {
		st.interactionID = interactionID
	}
	if st.interactionID == "" {
		st.interactionID = newInteractionID()
	}
	st.inputQueue = append(st.inputQueue, input)
	st.conversation = append(st.conversation, conversationMessage{
		Seq:             len(st.conversation) + 1,
		Role:            "user",
		Content:         input,
		InteractionID:   st.interactionID,
		CreatedAtUnixMs: time.Now().UnixMilli(),
	})
	st.waiting = false
	st.updatedAt = time.Now()
	snapshot = s.buildConversationSnapshotLocked(runID, st)
	notifyLocked(st)
	s.mu.Unlock()

	s.persistConversation(ctx, runID, snapshot)
	return &insightifyv1.SendResponse{
		Accepted:         true,
		InteractionId:    st.interactionID,
		AssistantMessage: "",
	}, nil
}
