package llm

import (
	"context"
	"encoding/json"

	llmclient "insightify/internal/llmClient"
)

type modelDispatchClient struct {
	fallback llmclient.LLMClient
}

func NewModelDispatchClient(fallback llmclient.LLMClient) llmclient.LLMClient {
	if fallback == nil {
		fallback = NewFakeClient(4096)
	}
	return &modelDispatchClient{fallback: fallback}
}

func (d *modelDispatchClient) Name() string { return "ModelDispatch:" + d.fallback.Name() }

func (d *modelDispatchClient) Close() error { return d.fallback.Close() }

func (d *modelDispatchClient) CountTokens(text string) int {
	return d.fallback.CountTokens(text)
}

func (d *modelDispatchClient) TokenCapacity() int {
	return d.fallback.TokenCapacity()
}

func (d *modelDispatchClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if sel, ok := selectedModelFrom(ctx); ok && sel.client != nil {
		return sel.client.GenerateJSON(ctx, prompt, input)
	}
	return d.fallback.GenerateJSON(ctx, prompt, input)
}

func (d *modelDispatchClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	if sel, ok := selectedModelFrom(ctx); ok && sel.client != nil {
		return sel.client.GenerateJSONStream(ctx, prompt, input, onChunk)
	}
	return d.fallback.GenerateJSONStream(ctx, prompt, input, onChunk)
}
