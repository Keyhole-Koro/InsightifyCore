package llmtool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"insightify/internal/mcp"
)

// PromptField describes a single output field in a simple schema.
type PromptField struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// PromptExample captures an optional input/output example.
type PromptExample struct {
	InputJSON  string
	OutputJSON string
}

// StructuredPromptSpec defines the sections for a structured prompt.
type StructuredPromptSpec struct {
	Purpose      string
	Background   string
	OutputFields []PromptField
	Constraints  []string
	Rules        []string
	Assumptions  []string
	OutputFormat string
	Language     string
	Examples     []PromptExample
}

// StructuredPromptBuilder renders a structured prompt including tool specs and MCP results.
func StructuredPromptBuilder(spec StructuredPromptSpec) PromptBuilder {
	return func(_ context.Context, state *ToolState, tools []mcp.ToolSpec) (string, error) {
		if strings.TrimSpace(spec.Purpose) == "" {
			return "", fmt.Errorf("llmtool: purpose is empty")
		}
		if len(spec.OutputFields) == 0 {
			return "", fmt.Errorf("llmtool: output fields are empty")
		}
		inputJSON, err := formatAnyJSON(state.Input)
		if err != nil {
			return "", fmt.Errorf("llmtool: encode input: %w", err)
		}

		var buf bytes.Buffer
		writeSection(&buf, "PURPOSE", spec.Purpose)
		writeSection(&buf, "BACKGROUND", spec.Background)
		writeSection(&buf, "INPUT", inputJSON)
		writeSection(&buf, "OUTPUT", formatFields(spec.OutputFields))
		writeSection(&buf, "CONSTRAINTS", formatList(spec.Constraints))
		writeSection(&buf, "RULES", formatList(spec.Rules))
		writeSection(&buf, "ASSUMPTIONS", formatList(spec.Assumptions))
		writeSection(&buf, "OUTPUT_FORMAT", spec.OutputFormat)
		writeSection(&buf, "LANGUAGE", spec.Language)
		writeSection(&buf, "TOOLS", FormatToolSpecs(tools))
		writeSection(&buf, "MCP_RESULTS", FormatToolResults(state.ToolResults))
		if len(spec.Examples) > 0 {
			writeSection(&buf, "EXAMPLES", formatExamples(spec.Examples))
		}

		return strings.TrimSpace(buf.String()) + "\n", nil
	}
}

func formatAnyJSON(v any) (string, error) {
	if v == nil {
		return "null", nil
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func formatFields(fields []PromptField) string {
	if len(fields) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, f := range fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			continue
		}
		req := "optional"
		if f.Required {
			req = "required"
		}
		if f.Description != "" {
			fmt.Fprintf(&buf, "- %s (%s, %s): %s\n", name, f.Type, req, f.Description)
		} else {
			fmt.Fprintf(&buf, "- %s (%s, %s)\n", name, f.Type, req)
		}
	}
	return strings.TrimRight(buf.String(), "\n")
}

func formatList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fmt.Fprintf(&buf, "- %s\n", item)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func formatExamples(examples []PromptExample) string {
	if len(examples) == 0 {
		return ""
	}
	var buf strings.Builder
	for i, ex := range examples {
		fmt.Fprintf(&buf, "Example %d:\n", i+1)
		if strings.TrimSpace(ex.InputJSON) != "" {
			buf.WriteString("INPUT:\n")
			buf.WriteString(ex.InputJSON)
			if !strings.HasSuffix(ex.InputJSON, "\n") {
				buf.WriteString("\n")
			}
		}
		if strings.TrimSpace(ex.OutputJSON) != "" {
			buf.WriteString("OUTPUT:\n")
			buf.WriteString(ex.OutputJSON)
			if !strings.HasSuffix(ex.OutputJSON, "\n") {
				buf.WriteString("\n")
			}
		}
		buf.WriteString("\n")
	}
	return strings.TrimRight(buf.String(), "\n")
}

func writeSection(buf *bytes.Buffer, title, body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	buf.WriteString("[")
	buf.WriteString(title)
	buf.WriteString("]\n")
	buf.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		buf.WriteString("\n")
	}
	buf.WriteString("\n")
}
