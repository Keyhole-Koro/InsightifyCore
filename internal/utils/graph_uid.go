package utils

import (
	"fmt"
	"strings"

	pipelinev1 "insightify/gen/go/pipeline/v1"
)

// AssignGraphNodeUIDs rewrites GraphNode.Uid values to generated UIDs and updates edges accordingly.
// It mutates the provided view in place and returns old->new UID mappings for non-empty original UIDs.
func AssignGraphNodeUIDs(view *pipelinev1.ClientView) map[string]string {
	return AssignGraphNodeUIDsWithGenerator(nil, view)
}

// AssignGraphNodeUIDsWithGenerator is AssignGraphNodeUIDs with an explicit shared generator.
func AssignGraphNodeUIDsWithGenerator(gen *UIDGenerator, view *pipelinev1.ClientView) map[string]string {
	if view == nil || view.GetGraph() == nil || len(view.GetGraph().GetNodes()) == 0 {
		return nil
	}
	if gen == nil {
		gen = NewUIDGenerator()
	}
	oldToNew := make(map[string]string, len(view.GetGraph().GetNodes()))
	idSeen := make(map[string]int, len(view.GetGraph().GetNodes()))

	for i, n := range view.GetGraph().GetNodes() {
		if n == nil {
			continue
		}
		seed := strings.TrimSpace(n.GetUid())
		if seed == "" {
			seed = strings.TrimSpace(n.GetLabel())
		}
		if seed == "" {
			seed = fmt.Sprintf("node-%d", i+1)
		}

		oldID := strings.TrimSpace(n.GetUid())
		key := ""
		if oldID != "" {
			idSeen[oldID]++
			if idSeen[oldID] == 1 {
				key = "id:" + oldID
			} else {
				key = fmt.Sprintf("id:%s#%d", oldID, idSeen[oldID])
			}
		} else {
			key = fmt.Sprintf("idx:%d|label:%s", i, strings.TrimSpace(n.GetLabel()))
		}

		uid := gen.GenerateForKey(key, seed)
		n.Uid = uid

		if oldID != "" {
			oldToNew[oldID] = uid
		}
	}

	for _, e := range view.GetGraph().GetEdges() {
		if e == nil {
			continue
		}
		if to, ok := oldToNew[e.From]; ok {
			e.From = to
		}
		if to, ok := oldToNew[e.To]; ok {
			e.To = to
		}
	}

	return oldToNew
}
