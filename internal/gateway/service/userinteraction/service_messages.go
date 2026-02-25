package userinteraction

import (
	"context"
	"fmt"
	"strings"
	"time"

	insightifyv1 "insightify/gen/go/insightify/v1"
	logctx "insightify/internal/common/logctx"
)

// PublishOutput enqueues a server assistant message for run+node.
func (s *Service) PublishOutput(ctx context.Context, runID, nodeID, interactionID, message string) error {
	runID = strings.TrimSpace(runID)
	nodeID = strings.TrimSpace(nodeID)
	interactionID = strings.TrimSpace(interactionID)
	message = strings.TrimSpace(message)
	if runID == "" || nodeID == "" {
		return fmt.Errorf("run_id and node_id are required")
	}
	if message == "" {
		return fmt.Errorf("message is required")
	}

	var (
		snapshot   []byte
		syncer     UISync
		syncRunID  string
		syncNodeID string
		syncInter  string
		syncOutput string
	)
	s.mu.Lock()

	st := s.getOrCreateLocked(runID, nodeID)
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
	snapshot = s.buildConversationSnapshotLocked(runID, nodeID, st)
	syncer = s.uiSync
	syncRunID = runID
	syncNodeID = nodeID
	syncInter = st.interactionID
	syncOutput = message
	notifyLocked(st)
	s.mu.Unlock()

	s.persistConversation(ctx, runID, nodeID, snapshot)
	if syncer != nil {
		_ = syncer.OnAssistantOutput(ctx, syncRunID, syncNodeID, syncInter, syncOutput)
	}
	logctx.Info(ctx, "interaction assistant output published", "run_id", runID, "node_id", nodeID, "interaction_id", st.interactionID)
	return nil
}

func (s *Service) Send(ctx context.Context, req *insightifyv1.SendRequest) (*insightifyv1.SendResponse, error) {
	runID := strings.TrimSpace(req.GetRunId())
	nodeID := strings.TrimSpace(req.GetNodeId())
	input := strings.TrimSpace(req.GetInput())
	if runID == "" || nodeID == "" {
		return nil, fmt.Errorf("run_id and node_id are required")
	}
	if input == "" {
		return nil, fmt.Errorf("input is required")
	}

	var (
		snapshot   []byte
		syncer     UISync
		syncRunID  string
		syncNodeID string
		syncInter  string
		syncInput  string
	)
	s.mu.Lock()

	st := s.getOrCreateLocked(runID, nodeID)
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
	snapshot = s.buildConversationSnapshotLocked(runID, nodeID, st)
	syncer = s.uiSync
	syncRunID = runID
	syncNodeID = nodeID
	syncInter = st.interactionID
	syncInput = input
	notifyLocked(st)
	s.mu.Unlock()

	s.persistConversation(ctx, runID, nodeID, snapshot)
	if syncer != nil {
		_ = syncer.OnUserAccepted(ctx, syncRunID, syncNodeID, syncInter, syncInput)
	}
	logctx.Info(ctx, "interaction user input accepted", "run_id", runID, "node_id", nodeID, "interaction_id", st.interactionID)
	return &insightifyv1.SendResponse{
		Accepted:         true,
		InteractionId:    st.interactionID,
		AssistantMessage: "",
	}, nil
}
