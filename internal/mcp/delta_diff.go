package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"insightify/internal/artifact"
	"insightify/internal/common/delta"
)

// --------------------- delta.diff ---------------------

type deltaDiffTool struct{}

func newDeltaDiffTool() *deltaDiffTool { return &deltaDiffTool{} }

func (t *deltaDiffTool) Spec() artifact.ToolSpec {
	return artifact.ToolSpec{
		Name:        "delta.diff",
		Description: "Compute a JSON delta between two values (before/after).",
	}
}

type deltaDiffInput struct {
	Before     json.RawMessage `json:"before"`
	After      json.RawMessage `json:"after"`
	MaxChanges int             `json:"max_changes,omitempty"`
}

func (t *deltaDiffTool) Call(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in deltaDiffInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}
	if len(in.Before) == 0 || len(in.After) == 0 {
		return nil, fmt.Errorf("delta.diff: before and after are required")
	}
	var before any
	if err := json.Unmarshal(in.Before, &before); err != nil {
		return nil, fmt.Errorf("delta.diff: decode before: %w", err)
	}
	var after any
	if err := json.Unmarshal(in.After, &after); err != nil {
		return nil, fmt.Errorf("delta.diff: decode after: %w", err)
	}
	out := delta.Diff(before, after, delta.Options{MaxChanges: in.MaxChanges})
	return json.Marshal(out)
}
