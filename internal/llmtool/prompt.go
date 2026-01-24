package llmtool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"insightify/internal/mcp"
)

// FormatToolSpecs renders a compact JSON block of tool specs for prompt inclusion.
func FormatToolSpecs(tools []mcp.ToolSpec) string {
	if tools == nil {
		tools = []mcp.ToolSpec{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(tools)
	return buf.String()
}

// FormatToolResults renders tool results as a JSON block.
func FormatToolResults(results []ToolResult) string {
	if results == nil {
		results = []ToolResult{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(results)
	return buf.String()
}

// DefaultPromptBuilder is a simple prompt builder for tool loop usage.
// It appends tool specs and previous tool results as JSON blocks.
func DefaultPromptBuilder(base string) PromptBuilder {
	return func(_ context.Context, state *ToolState, tools []mcp.ToolSpec) (string, error) {
		if base == "" {
			return "", fmt.Errorf("llmtool: base prompt is empty")
		}
		var buf bytes.Buffer
		buf.WriteString(base)
		buf.WriteString("\n\n[TOOLS]\n")
		buf.WriteString(FormatToolSpecs(tools))
		if len(state.ToolResults) > 0 {
			buf.WriteString("\n[TOOL_RESULTS]\n")
			buf.WriteString(FormatToolResults(state.ToolResults))
		}
		return buf.String(), nil
	}
}
