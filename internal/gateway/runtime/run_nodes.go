package runtime

import (
	"strings"

	"google.golang.org/protobuf/proto"
	insightifyv1 "insightify/gen/go/insightify/v1"
)

func (a *App) SetRunNode(runID string, node *insightifyv1.UiNode) {
	runID = strings.TrimSpace(runID)
	if a == nil || runID == "" || node == nil {
		return
	}
	a.runNodeMu.Lock()
	a.runNodes[runID] = node
	a.runNodeMu.Unlock()
}

func (a *App) GetRunNode(runID string) *insightifyv1.UiNode {
	runID = strings.TrimSpace(runID)
	if a == nil || runID == "" {
		return nil
	}
	a.runNodeMu.RLock()
	node := a.runNodes[runID]
	a.runNodeMu.RUnlock()
	if node == nil {
		return nil
	}
	cloned, ok := proto.Clone(node).(*insightifyv1.UiNode)
	if !ok {
		return nil
	}
	return cloned
}

func (a *App) ClearRunNode(runID string) {
	runID = strings.TrimSpace(runID)
	if a == nil || runID == "" {
		return
	}
	a.runNodeMu.Lock()
	delete(a.runNodes, runID)
	a.runNodeMu.Unlock()
}
