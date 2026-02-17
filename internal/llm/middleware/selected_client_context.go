package llm

import (
	"context"

	llmclient "insightify/internal/llm/client"
)

type ctxKeySelectedClient struct{}

// WithSelectedClient stores the selected model client in context.
func WithSelectedClient(ctx context.Context, client llmclient.LLMClient) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeySelectedClient{}, client)
}

// SelectedClientFrom extracts the selected model client from context.
func SelectedClientFrom(ctx context.Context) (llmclient.LLMClient, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(ctxKeySelectedClient{})
	if v == nil {
		return nil, false
	}
	client, ok := v.(llmclient.LLMClient)
	if !ok || client == nil {
		return nil, false
	}
	return client, true
}
