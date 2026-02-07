package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	llmclient "insightify/internal/llmClient"
)

type ctxKeySelectedModel struct{}

type selectedModel struct {
	client llmclient.LLMClient
}

func withSelectedModel(ctx context.Context, sel selectedModel) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeySelectedModel{}, sel)
}

func selectedModelFrom(ctx context.Context) (selectedModel, bool) {
	if ctx == nil {
		return selectedModel{}, false
	}
	if v := ctx.Value(ctxKeySelectedModel{}); v != nil {
		if sel, ok := v.(selectedModel); ok {
			return sel, true
		}
	}
	return selectedModel{}, false
}

func SelectModel(reg *InMemoryModelRegistry, tokenCap int) Middleware {
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &modelSelecting{
			next:     next,
			registry: reg,
			tokenCap: tokenCap,
			clients:  map[string]selectedModel{},
		}
	}
}

type modelSelecting struct {
	next     llmclient.LLMClient
	registry *InMemoryModelRegistry
	tokenCap int

	mu      sync.Mutex
	clients map[string]selectedModel
}

func (m *modelSelecting) Name() string { return m.next.Name() }

func (m *modelSelecting) Close() error {
	seen := map[llmclient.LLMClient]struct{}{}
	for _, sel := range m.clients {
		if _, ok := seen[sel.client]; ok {
			continue
		}
		seen[sel.client] = struct{}{}
		_ = sel.client.Close()
	}
	return m.next.Close()
}

func (m *modelSelecting) CountTokens(text string) int {
	return m.next.CountTokens(text)
}

func (m *modelSelecting) TokenCapacity() int {
	return m.next.TokenCapacity()
}

func (m *modelSelecting) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	sel, err := m.resolve(ctx)
	if err != nil {
		return nil, err
	}
	ctx = withSelectedModel(ctx, sel)
	return m.next.GenerateJSON(ctx, prompt, input)
}

func (m *modelSelecting) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	sel, err := m.resolve(ctx)
	if err != nil {
		return nil, err
	}
	ctx = withSelectedModel(ctx, sel)
	return m.next.GenerateJSONStream(ctx, prompt, input, onChunk)
}

func (m *modelSelecting) resolve(ctx context.Context) (selectedModel, error) {
	if m.registry == nil {
		return selectedModel{}, fmt.Errorf("model registry is nil")
	}
	role := ModelRoleFrom(ctx)
	level := ModelLevelFrom(ctx)
	provider := ModelProviderFrom(ctx)
	model := ModelNameFrom(ctx)
	if normalizeLevel(level) == "" {
		return selectedModel{}, ErrModelLevelRequired
	}

	entry, err := m.registry.Resolve(role, level, provider, model)
	if err != nil {
		return selectedModel{}, err
	}
	k := fmt.Sprintf("%s|%s|%s|%d", role, level, keyFor(entry.Profile.Provider, entry.Profile.Model), m.tokenCap)

	m.mu.Lock()
	defer m.mu.Unlock()
	if sel, ok := m.clients[k]; ok {
		return sel, nil
	}
	cli, err := m.registry.BuildClient(ctx, role, level, provider, model, m.tokenCap)
	if err != nil {
		return selectedModel{}, err
	}
	sel := selectedModel{client: cli}
	m.clients[k] = sel
	return sel, nil
}
