package ui

import (
	"context"
	"strings"

	insightifyv1 "insightify/gen/go/insightify/v1"
)

// Store defines operations for fetching and updating UI document state.
type Store interface {
	GetDocument(ctx context.Context, runID string) (*insightifyv1.UiDocument, error)
	ApplyOps(ctx context.Context, runID string, baseVersion int64, ops []*insightifyv1.UiOp) (*insightifyv1.UiDocument, bool, error)
}

func normalizeRunID(runID string) string {
	return strings.TrimSpace(runID)
}
