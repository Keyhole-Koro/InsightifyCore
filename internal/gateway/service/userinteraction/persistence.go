package userinteraction

import (
	"context"
	"encoding/json"
	"log"
	"strings"
)

func (s *Service) buildConversationSnapshotLocked(runID string, st *sessionState) []byte {
	if s == nil || s.artifact == nil || st == nil {
		return nil
	}
	doc := conversationArtifact{
		RunID:    runID,
		Messages: append([]conversationMessage(nil), st.conversation...),
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		log.Printf("conversation snapshot marshal failed for run %s: %v", runID, err)
		return nil
	}
	return raw
}

func (s *Service) persistConversation(ctx context.Context, runID string, raw []byte) {
	if s == nil || s.artifact == nil || len(raw) == 0 || strings.TrimSpace(runID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.artifact.Put(ctx, runID, s.conversationArtifactPath, raw); err != nil {
		log.Printf("persist conversation artifact failed for run %s: %v", runID, err)
	}
}
