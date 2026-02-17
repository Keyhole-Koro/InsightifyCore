package model

import (
	"context"
	"encoding/json"
	"testing"

	llmclient "insightify/internal/llm/client"
	llmmiddleware "insightify/internal/llm/middleware"
)

type awareTestLLM struct {
	name     string
	tokenCap int
	headers  llmclient.RateLimitHeaders
	has      bool
}

func (t *awareTestLLM) Name() string { return t.name }
func (t *awareTestLLM) Close() error { return nil }
func (t *awareTestLLM) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(text) / 3
}
func (t *awareTestLLM) TokenCapacity() int { return t.tokenCap }
func (t *awareTestLLM) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	_ = ctx
	_ = prompt
	_ = input
	return json.RawMessage(`{"model":"` + t.name + `"}`), nil
}
func (t *awareTestLLM) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	raw, err := t.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return nil, err
	}
	if onChunk != nil {
		onChunk(string(raw))
	}
	return raw, nil
}
func (t *awareTestLLM) SetRateLimitHeaderHandler(handler llmclient.RateLimitHeaderHandler) {
	_ = handler
}
func (t *awareTestLLM) LastRateLimitHeaders() (llmclient.RateLimitHeaders, bool) {
	return t.headers, t.has
}

var _ llmclient.RateLimitHeaderAwareClient = (*awareTestLLM)(nil)

func TestSelectModel_DefaultModePrefersAvailableHeaders(t *testing.T) {
	reg := NewInMemoryModelRegistry()

	registerAware := func(provider, model string, remainingTokens int) {
		t.Helper()
		err := reg.RegisterModel(llmclient.ModelRegistration{
			Provider: provider,
			Model:    model,
			Level:    llmclient.ModelLevelMiddle,
			Factory: func(ctx context.Context, tokenCap int) (llmclient.LLMClient, error) {
				_ = ctx
				if tokenCap <= 0 {
					tokenCap = 1024
				}
				return &awareTestLLM{
					name:     provider + ":" + model,
					tokenCap: tokenCap,
					headers:  llmclient.RateLimitHeaders{RemainingTokens: remainingTokens},
					has:      true,
				}, nil
			},
		})
		if err != nil {
			t.Fatalf("register %s:%s: %v", provider, model, err)
		}
	}

	registerAware("a", "m-default", 100)
	registerAware("b", "m-roomy", 900)

	if err := reg.SetDefault(ModelRoleWorker, ModelLevelMiddle, "a", "m-default"); err != nil {
		t.Fatalf("set default: %v", err)
	}

	fallback := &awareTestLLM{name: "fallback", tokenCap: 4096} // Test
	client := llmmiddleware.Wrap(NewModelDispatchClient(fallback),
		SelectModel(reg, 4096, ModelSelectionModePreferAvailable),
	)

	ctx := WithModelSelection(context.Background(), ModelRoleWorker, ModelLevelMiddle, "", "")
	raw, err := client.GenerateJSON(ctx, "p", nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if string(raw) != `{"model":"b:m-roomy"}` {
		t.Fatalf("unexpected model: %s", string(raw))
	}
}
