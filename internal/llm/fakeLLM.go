package llm

import (
	"context"
	"encoding/json"
)

// FakeClient returns deterministic, minimal JSON payloads per phase for offline/testing.
type FakeClient struct {
	tokenCap int
}

func NewFakeClient(cap int) *FakeClient {
	if cap <= 0 {
		cap = 4096
	}
	return &FakeClient{tokenCap: cap}
}
func (f *FakeClient) Name() string { return "FakeLLM" }
func (f *FakeClient) Close() error { return nil }
func (f *FakeClient) CountTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}
func (f *FakeClient) TokenCapacity() int { return f.tokenCap }

func (f *FakeClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	phase := PhaseFrom(ctx)
	var obj any
	switch phase {
	case "m0":
		obj = map[string]any{
			"main_source_roots":    []string{"src", "internal"},
			"library_roots":        []string{"third_party", "vendor"},
			"config_roots":         []string{".github", "scripts"},
			"config_files":         []string{".env.example"},
			"runtime_config_files": []string{},
			"notes":                []string{"fake m0 output"},
		}
	case "m1":
		obj = map[string]any{
			"architecture_hypothesis": map[string]any{
				"purpose":         "fake purpose",
				"summary":         "fake summary",
				"key_components":  []any{},
				"execution_model": "fake",
				"tech_stack": map[string]any{
					"platforms":   []string{},
					"languages":   []string{"Go"},
					"build_tools": []string{"go"},
				},
				"assumptions": []string{},
				"unknowns":    []string{},
				"confidence":  0.5,
			},
			"next_files":     []any{},
			"next_patterns":  []any{},
			"contradictions": []any{},
			"needs_input":    []string{},
			"stop_when":      []string{},
			"notes":          []string{"fake m1 output"},
		}
	case "m2":
		obj = map[string]any{
			"updated_hypothesis": map[string]any{
				"purpose":         "fake purpose",
				"summary":         "fake summary",
				"key_components":  []any{},
				"execution_model": "fake",
				"tech_stack": map[string]any{
					"platforms":   []string{},
					"languages":   []string{"Go"},
					"build_tools": []string{"go"},
				},
				"assumptions":          []string{},
				"unknowns":             []string{},
				"confidence":           0.5,
				"verification_targets": []any{},
			},
			"question_status": []any{},
			"delta":           map[string]any{"added": []string{}, "removed": []string{}, "modified": []any{}},
			"contradictions":  []any{},
			"next_files":      []any{},
			"next_patterns":   []any{},
			"needs_input":     []string{},
			"stop_when":       []string{},
			"notes":           []string{"fake m2 output"},
		}
	case "x0":
		obj = map[string]any{
			"external_overview": map[string]any{
				"purpose":              "fake external summary",
				"architecture_summary": "fake architecture summary",
				"external_systems": []any{
					map[string]any{
						"name":        "FakeAPI",
						"kind":        "REST",
						"interaction": "calls FakeAPI for demo",
						"evidence":    []any{},
						"confidence":  0.5,
					},
				},
				"infra_components": []any{},
				"build_and_deploy": []any{},
				"runtime_configs":  []any{},
				"confidence":       0.5,
			},
			"evidence_gaps": []any{},
			"notes":         []string{"fake x0 output"},
		}
	case "x1":
		obj = map[string]any{
			"delta": map[string]any{
				"added":   []string{},
				"removed": []string{},
				"modified": []any{
					map[string]any{
						"field":  "external_overview.confidence",
						"before": "0.5",
						"after":  "0.75",
					},
				},
			},
			"needs_input": []string{},
			"stop_when":   []string{},
			"notes":       []string{"fake x1 output"},
		}
	default:
		// generic empty JSON object
		obj = map[string]any{}
	}
	b, _ := json.Marshal(obj)
	return json.RawMessage(b), nil
}
