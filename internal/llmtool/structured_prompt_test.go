package llmtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"insightify/internal/mcp"
)

func TestStructuredPromptBuilder_RendersSections(t *testing.T) {
	spec := StructuredPromptSpec{
		Purpose:      "Summarize repository context.",
		Background:   "Phase M0 - overview.",
		OutputFormat: "JSON only.",
		Language:     "English",
		OutputFields: []PromptField{
			{Name: "summary", Type: "string", Required: true, Description: "Short summary."},
			{Name: "risks", Type: "[]string", Required: false},
		},
		Constraints: []string{"No markdown."},
		Rules:       []string{"Be concise."},
		Assumptions: []string{"If unsure, return empty strings."},
		Examples: []PromptExample{
			{InputJSON: `{"repo":"x"}`, OutputJSON: `{"summary":"ok"}`},
		},
	}

	state := &ToolState{
		Input: map[string]any{"repo": "demo"},
		ToolResults: []ToolResult{
			{Name: "scan.list", Input: json.RawMessage(`{"roots":["."]}`), Output: json.RawMessage(`{"files":["a.go"]}`)},
		},
	}
	tools := []mcp.ToolSpec{{Name: "scan.list"}}

	builder := StructuredPromptBuilder(spec)
	out, err := builder(context.Background(), state, tools)
	if err != nil {
		t.Fatalf("builder error: %v", err)
	}

	wantSections := []string{
		"[PURPOSE]",
		"[BACKGROUND]",
		"[INPUT]",
		"[OUTPUT]",
		"[CONSTRAINTS]",
		"[RULES]",
		"[ASSUMPTIONS]",
		"[OUTPUT_FORMAT]",
		"[LANGUAGE]",
		"[TOOLS]",
		"[MCP_RESULTS]",
		"[EXAMPLES]",
	}
	for _, sec := range wantSections {
		if !strings.Contains(out, sec) {
			t.Fatalf("expected section %s in prompt", sec)
		}
	}
}

func TestStructuredPromptBuilder_RequiresPurpose(t *testing.T) {
	spec := StructuredPromptSpec{
		OutputFields: []PromptField{{Name: "summary", Type: "string", Required: true}},
	}
	builder := StructuredPromptBuilder(spec)
	_, err := builder(context.Background(), &ToolState{Input: map[string]any{}}, nil)
	if err == nil || !strings.Contains(err.Error(), "purpose") {
		t.Fatalf("expected purpose error, got %v", err)
	}
}

func TestStructuredPromptBuilder_RequiresOutputFields(t *testing.T) {
	spec := StructuredPromptSpec{Purpose: "x"}
	builder := StructuredPromptBuilder(spec)
	_, err := builder(context.Background(), &ToolState{Input: map[string]any{}}, nil)
	if err == nil || !strings.Contains(err.Error(), "output fields") {
		t.Fatalf("expected output fields error, got %v", err)
	}
}

func TestApplyPresets_PrependConstraintsAndRules(t *testing.T) {
	spec := StructuredPromptSpec{
		Purpose:      "x",
		OutputFields: []PromptField{{Name: "summary", Type: "string", Required: true}},
		Constraints:  []string{"spec-constraint"},
		Rules:        []string{"spec-rule"},
	}
	preset := PromptPreset{
		Constraints: []string{"preset-constraint"},
		Rules:       []string{"preset-rule"},
	}
	applied := ApplyPresets(spec, preset)
	if len(applied.Constraints) < 2 || applied.Constraints[0] != "preset-constraint" {
		t.Fatalf("expected preset constraint prepended, got %+v", applied.Constraints)
	}
	if len(applied.Rules) < 2 || applied.Rules[0] != "preset-rule" {
		t.Fatalf("expected preset rule prepended, got %+v", applied.Rules)
	}
}
