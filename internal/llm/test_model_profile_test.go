package llm

import (
	"context"
	"encoding/json"
	"testing"

	llmclient "insightify/internal/llmclient"
)

type testLLM struct {
	name     string
	tokenCap int
}

func (t *testLLM) Name() string { return t.name }
func (t *testLLM) Close() error { return nil }
func (t *testLLM) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 3
}
func (t *testLLM) TokenCapacity() int { return t.tokenCap }
func (t *testLLM) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	_ = ctx
	_ = prompt
	_ = input
	return json.RawMessage(`{"model":"` + t.name + `"}`), nil
}
func (t *testLLM) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	raw, err := t.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return nil, err
	}
	if onChunk != nil {
		onChunk(string(raw))
	}
	return raw, nil
}

var _ llmclient.LLMClient = (*testLLM)(nil)

func registerTestModel(t *testing.T, reg *InMemoryModelRegistry, provider, model string, level llmclient.ModelLevel) {
	t.Helper()
	if err := reg.RegisterModel(llmclient.ModelRegistration{
		Provider: provider,
		Model:    model,
		Level:    level,
		Factory: func(ctx context.Context, tokenCap int) (llmclient.LLMClient, error) {
			_ = ctx
			if tokenCap <= 0 {
				tokenCap = 1024
			}
			return &testLLM{name: provider + ":" + model, tokenCap: tokenCap}, nil
		},
	}); err != nil {
		t.Fatalf("register %s:%s: %v", provider, model, err)
	}
}

func TestRegistryResolve_DefaultByRoleAndLevel(t *testing.T) {
	reg := NewInMemoryModelRegistry()
	registerTestModel(t, reg, "a", "m-low", llmclient.ModelLevelLow)
	registerTestModel(t, reg, "b", "m-high", llmclient.ModelLevelHigh)

	if err := reg.SetDefault(ModelRoleWorker, ModelLevelLow, "a", "m-low"); err != nil {
		t.Fatalf("set default worker low: %v", err)
	}
	if err := reg.SetDefault(ModelRolePlanner, ModelLevelHigh, "b", "m-high"); err != nil {
		t.Fatalf("set default planner high: %v", err)
	}

	m, err := reg.Resolve(ModelRolePlanner, ModelLevelHigh, "", "")
	if err != nil {
		t.Fatalf("resolve planner high: %v", err)
	}
	if m.Profile.Provider != "b" || m.Profile.Model != "m-high" {
		t.Fatalf("unexpected model: provider=%s model=%s", m.Profile.Provider, m.Profile.Model)
	}
}

func TestSelectModelMiddleware_UsesOverrideProviderModel(t *testing.T) {
	reg := NewInMemoryModelRegistry()
	registerTestModel(t, reg, "a", "m-mid", llmclient.ModelLevelMiddle)
	registerTestModel(t, reg, "b", "m-high", llmclient.ModelLevelHigh)

	if err := reg.SetDefault(ModelRoleWorker, ModelLevelMiddle, "a", "m-mid"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if err := reg.SetDefault(ModelRolePlanner, ModelLevelHigh, "b", "m-high"); err != nil {
		t.Fatalf("set default: %v", err)
	}

	fallback := &testLLM{name: "fallback", tokenCap: 4096}
	// Test
	client := Wrap(NewModelDispatchClient(fallback),
		SelectModel(reg, 4096, ModelSelectionModePreferAvailable),
	)

	ctx := WithModelSelection(context.Background(), ModelRolePlanner, ModelLevelHigh, "", "")
	raw, err := client.GenerateJSON(ctx, "p", nil)
	if err != nil {
		t.Fatalf("generate planner high: %v", err)
	}
	if string(raw) != `{"model":"b:m-high"}` {
		t.Fatalf("unexpected selected model: %s", string(raw))
	}

	ctx = WithModelSelection(context.Background(), ModelRoleWorker, ModelLevelMiddle, "a", "m-mid")
	raw, err = client.GenerateJSON(ctx, "p", nil)
	if err != nil {
		t.Fatalf("generate worker mid override: %v", err)
	}
	if string(raw) != `{"model":"a:m-mid"}` {
		t.Fatalf("unexpected override model: %s", string(raw))
	}
}
