package ui

import (
	"context"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	actdomain "insightify/internal/domain/act"
)

func (s *Service) withConversationHistory(ctx context.Context, runID string, doc *insightifyv1.UiDocument) *insightifyv1.UiDocument {
	if s == nil || s.artifact == nil || doc == nil {
		return doc
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return doc
	}
	out := cloneDocument(doc)
	for _, node := range out.GetNodes() {
		if node == nil {
			continue
		}
		if node.GetType() == insightifyv1.UiNodeType_UI_NODE_TYPE_ACT && node.GetAct() != nil {
			node.Act = actdomain.NormalizeActState(node.GetAct())
		}
	}
	return out
}

func cloneDocument(in *insightifyv1.UiDocument) *insightifyv1.UiDocument {
	if in == nil {
		return nil
	}
	out := &insightifyv1.UiDocument{
		RunId:   in.GetRunId(),
		Version: in.GetVersion(),
	}
	nodes := in.GetNodes()
	if len(nodes) == 0 {
		return out
	}
	out.Nodes = append([]*insightifyv1.UiNode(nil), nodes...)
	return out
}
