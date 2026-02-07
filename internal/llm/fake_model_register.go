package llm

import (
	"context"

	llmclient "insightify/internal/llmClient"
)

func RegisterFakeModels(reg llmclient.ModelRegistrar) error {
	type fakeModel struct {
		name   string
		level  llmclient.ModelLevel
		tokens int
		params int64
	}
	models := []fakeModel{
		{name: "fake-low", level: llmclient.ModelLevelLow, tokens: 2048, params: 1_000_000_000},
		{name: "fake-middle", level: llmclient.ModelLevelMiddle, tokens: 4096, params: 7_000_000_000},
		{name: "fake-high", level: llmclient.ModelLevelHigh, tokens: 8192, params: 30_000_000_000},
		{name: "fake-xhigh", level: llmclient.ModelLevelXHigh, tokens: 16384, params: 70_000_000_000},
	}
	for _, m := range models {
		name := m.name
		tokens := m.tokens
		params := m.params
		level := m.level
		if err := reg.RegisterModel(llmclient.ModelRegistration{
			Provider:       "fake",
			Model:          name,
			Level:          level,
			MaxTokens:      tokens,
			ParameterCount: params,
			Factory: func(ctx context.Context, tokenCap int) (llmclient.LLMClient, error) {
				_ = ctx
				if tokenCap <= 0 {
					tokenCap = tokens
				}
				return NewFakeClient(tokenCap), nil
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func FakeModelByLevel(level ModelLevel) string {
	switch level {
	case ModelLevelLow:
		return "fake-low"
	case ModelLevelHigh:
		return "fake-high"
	case ModelLevelXHigh:
		return "fake-xhigh"
	default:
		return "fake-middle"
	}
}
