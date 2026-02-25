package userinteraction

import (
	"context"
	"encoding/json"
	"log"
	"strings"
)

func (s *Service) conversationArtifactPathForNode(nodeID string) string {
	base := strings.TrimSpace(s.conversationArtifactPath)
	if base == "" {
		base = defaultConversationArtifactPath
	}
	return strings.TrimSpace(nodeID) + "/" + base
}

func (s *Service) buildConversationSnapshotLocked(runID, nodeID string, st *sessionState) []byte {
	if s == nil || s.artifact == nil || st == nil {
		return nil
	}
	doc := conversationArtifact{
		RunID:    runID,
		NodeID:   nodeID,
		Messages: append([]conversationMessage(nil), st.conversation...),
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		log.Printf("conversation snapshot marshal failed for run %s node %s: %v", runID, nodeID, err)
		return nil
	}
	return raw
}

func (s *Service) persistConversation(ctx context.Context, runID, nodeID string, raw []byte) {
	if s == nil || s.artifact == nil || len(raw) == 0 || strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	path := s.conversationArtifactPathForNode(nodeID)
	if err := s.artifact.Put(ctx, runID, path, raw); err != nil {
		log.Printf("persist conversation artifact failed for run %s node %s: %v", runID, nodeID, err)
	}
}
