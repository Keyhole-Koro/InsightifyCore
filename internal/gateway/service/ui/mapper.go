package ui

import (
	"fmt"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
	workerv1 "insightify/gen/go/worker/v1"
)

func (s *Service) UpsertFromClientView(runID, workerID string, view *workerv1.ClientView) *insightifyv1.UiNode {
	node := nodeFromClientView(workerID, view)
	if node == nil {
		return nil
	}
	s.Set(runID, node)
	return node
}

func nodeFromClientView(workerID string, view *workerv1.ClientView) *insightifyv1.UiNode {
	if view == nil {
		return nil
	}
	content := strings.TrimSpace(view.GetLlmResponse())
	if content == "" {
		return nil
	}
	return &insightifyv1.UiNode{
		Id:   fmt.Sprintf("%s-node", sanitizeID(workerID)),
		Type: insightifyv1.UiNodeType_UI_NODE_TYPE_ACT,
		Meta: &insightifyv1.UiNodeMeta{
			Title: strings.TrimSpace(workerID),
		},
		Act: &insightifyv1.UiActState{
			ActId:  fmt.Sprintf("%s-node", sanitizeID(workerID)),
			Status: insightifyv1.UiActStatus_UI_ACT_STATUS_DONE,
			Mode:   "done",
			Timeline: []*insightifyv1.UiActTimelineEvent{
				{
					Id:              fmt.Sprintf("%s-assistant-1", sanitizeID(workerID)),
					CreatedAtUnixMs: 0,
					Kind:            "worker_output",
					Summary:         content,
					WorkerKey:       strings.TrimSpace(workerID),
				},
			},
		},
	}
}

func sanitizeID(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "unknown"
	}
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "/", "_")
	return v
}
