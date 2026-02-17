package llmtool

import (
	"context"
	"encoding/json"
	"testing"

	"insightify/internal/artifact"
)

type fakeLLM struct {
	responses []json.RawMessage
}

func (f *fakeLLM) Name() string                { return "fake" }
func (f *fakeLLM) Close() error                { return nil }
func (f *fakeLLM) CountTokens(text string) int { return len(text) }
func (f *fakeLLM) TokenCapacity() int          { return 1000 }
func (f *fakeLLM) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if len(f.responses) == 0 {
		return nil, nil
	}
	out := f.responses[0]
	f.responses = f.responses[1:]
	return out, nil
}
func (f *fakeLLM) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	return f.GenerateJSON(ctx, prompt, input)
}

type fakeTools struct {
	specs []artifact.ToolSpec
	calls []string
}

func (f *fakeTools) Specs() []artifact.ToolSpec { return f.specs }
func (f *fakeTools) Call(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
	f.calls = append(f.calls, name)
	return json.RawMessage(`{"ok":true}`), nil
}

func TestToolLoop_ToolThenFinal(t *testing.T) {
	llm := &fakeLLM{
		responses: []json.RawMessage{
			json.RawMessage(`{"action":"tool","tool_name":"scan.list","tool_input":{"roots":["."]}}`),
			json.RawMessage(`{"action":"final","final":{"result":"done"}}`),
		},
	}
	tools := &fakeTools{specs: []artifact.ToolSpec{{Name: "scan.list"}}}
	loop := &ToolLoop{LLM: llm, Tools: tools, MaxIters: 3}
	out, state, err := loop.Run(context.Background(), map[string]any{"x": 1}, DefaultPromptBuilder("base"))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if state == nil || len(state.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %+v", state)
	}
	if string(out) != `{"result":"done"}` {
		t.Fatalf("unexpected final: %s", string(out))
	}
}

func TestToolLoop_AllowedList(t *testing.T) {
	llm := &fakeLLM{
		responses: []json.RawMessage{
			json.RawMessage(`{"action":"tool","tool_name":"fs.read","tool_input":{"path":"x"}}`),
		},
	}
	tools := &fakeTools{specs: []artifact.ToolSpec{{Name: "fs.read"}}}
	loop := &ToolLoop{LLM: llm, Tools: tools, MaxIters: 1, Allowed: []string{"scan.list"}}
	_, _, err := loop.Run(context.Background(), nil, DefaultPromptBuilder("base"))
	if err != ErrToolNotAllowed {
		t.Fatalf("expected ErrToolNotAllowed, got %v", err)
	}
}

func TestToolLoop_MaxIterations(t *testing.T) {
	llm := &fakeLLM{
		responses: []json.RawMessage{
			json.RawMessage(`{"action":"tool","tool_name":"scan.list","tool_input":{"roots":["."]}}`),
			json.RawMessage(`{"action":"tool","tool_name":"scan.list","tool_input":{"roots":["."]}}`),
		},
	}
	tools := &fakeTools{specs: []artifact.ToolSpec{{Name: "scan.list"}}}
	loop := &ToolLoop{LLM: llm, Tools: tools, MaxIters: 1}
	_, _, err := loop.Run(context.Background(), nil, DefaultPromptBuilder("base"))
	if err != ErrMaxIterations {
		t.Fatalf("expected ErrMaxIterations, got %v", err)
	}
}
