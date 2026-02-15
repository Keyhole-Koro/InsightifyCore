package ui

import "insightify/gen/go/insightify/v1"

// Store defines operations for fetching and updating UI document state.
type Store interface {
	GetDocument(runID string) *insightifyv1.UiDocument
	ApplyOps(runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error)
}
