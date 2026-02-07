package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"

	"insightify/internal/globalctx"
	llmclient "insightify/internal/llmClient"
)

// ----------------------------------------------------------------------------
// Model context – context keys for model selection
// ----------------------------------------------------------------------------

type ctxKeyModelRole struct{}
type ctxKeyModelLevel struct{}
type ctxKeyModelProvider struct{}
type ctxKeyModelName struct{}

// WithModelRole attaches a model role to the context.
func WithModelRole(ctx context.Context, role ModelRole) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyModelRole{}, normalizeRole(role))
}

// WithModelLevel attaches a model level to the context.
func WithModelLevel(ctx context.Context, level ModelLevel) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyModelLevel{}, normalizeLevel(level))
}

// WithModelProvider attaches a model provider to the context.
func WithModelProvider(ctx context.Context, provider string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyModelProvider{}, strings.ToLower(strings.TrimSpace(provider)))
}

// WithModelName attaches a model name to the context.
func WithModelName(ctx context.Context, model string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyModelName{}, strings.TrimSpace(model))
}

// WithModelSelection attaches all model selection parameters to the context.
func WithModelSelection(ctx context.Context, role ModelRole, level ModelLevel, provider, model string) context.Context {
	ctx = WithModelRole(ctx, role)
	ctx = WithModelLevel(ctx, level)
	ctx = WithModelProvider(ctx, provider)
	return WithModelName(ctx, model)
}

// ModelRoleFrom extracts the model role from the context.
func ModelRoleFrom(ctx context.Context) ModelRole {
	if ctx != nil {
		if v := ctx.Value(ctxKeyModelRole{}); v != nil {
			if role, ok := v.(ModelRole); ok {
				return normalizeRole(role)
			}
		}
	}
	return ModelRoleWorker
}

// ModelLevelFrom extracts the model level from the context.
func ModelLevelFrom(ctx context.Context) ModelLevel {
	if ctx != nil {
		if v := ctx.Value(ctxKeyModelLevel{}); v != nil {
			if level, ok := v.(ModelLevel); ok {
				return normalizeLevel(level)
			}
		}
	}
	return ""
}

// ModelProviderFrom extracts the model provider from the context.
func ModelProviderFrom(ctx context.Context) string {
	if ctx != nil {
		if v := ctx.Value(ctxKeyModelProvider{}); v != nil {
			if provider, ok := v.(string); ok {
				return strings.ToLower(strings.TrimSpace(provider))
			}
		}
	}
	return ""
}

// ModelNameFrom extracts the model name from the context.
func ModelNameFrom(ctx context.Context) string {
	if ctx != nil {
		if v := ctx.Value(ctxKeyModelName{}); v != nil {
			if model, ok := v.(string); ok {
				return strings.TrimSpace(model)
			}
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// Selected model – stored in context after selection
// ----------------------------------------------------------------------------

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

// ----------------------------------------------------------------------------
// SelectModel middleware
// ----------------------------------------------------------------------------

// SelectModel returns a middleware that resolves and caches model clients.
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
	mode := globalctx.ModelSelectionModeFrom(ctx)
	if normalizeLevel(level) == "" {
		return selectedModel{}, ErrModelLevelRequired
	}

	if mode == globalctx.ModelSelectionModePreferAvailable && provider == "" && model == "" {
		return m.resolvePreferAvailable(ctx, role, level)
	}
	entry, err := m.registry.Resolve(role, level, provider, model)
	if err != nil {
		return selectedModel{}, err
	}
	return m.getOrCreateSelected(ctx, role, level, entry)
}

func (m *modelSelecting) selectionCacheKey(role ModelRole, level ModelLevel, provider, model string) string {
	return fmt.Sprintf("%s|%s|%s|%d", role, level, keyFor(provider, model), m.tokenCap)
}

func (m *modelSelecting) getOrCreateSelected(ctx context.Context, role ModelRole, level ModelLevel, entry RegisteredModel) (selectedModel, error) {
	k := m.selectionCacheKey(role, level, entry.Profile.Provider, entry.Profile.Model)
	m.mu.Lock()
	defer m.mu.Unlock()
	if sel, ok := m.clients[k]; ok {
		return sel, nil
	}
	cli, err := m.registry.BuildClient(ctx, role, level, entry.Profile.Provider, entry.Profile.Model, m.tokenCap)
	if err != nil {
		return selectedModel{}, err
	}
	sel := selectedModel{client: cli}
	m.clients[k] = sel
	return sel, nil
}

func (m *modelSelecting) resolvePreferAvailable(ctx context.Context, role ModelRole, level ModelLevel) (selectedModel, error) {
	candidates := m.registry.Candidates(role, level)
	if len(candidates) == 0 {
		return selectedModel{}, fmt.Errorf("%w: role=%s level=%s", ErrModelNotRegistered, role, level)
	}

	bestIdx := 0
	bestScore := math.Inf(-1)
	for i, entry := range candidates {
		sel, err := m.getOrCreateSelected(ctx, role, level, entry)
		if err != nil {
			continue
		}
		score := availabilityScore(sel.client)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return m.getOrCreateSelected(ctx, role, level, candidates[bestIdx])
}

func availabilityScore(cli llmclient.LLMClient) float64 {
	aware, ok := cli.(llmclient.RateLimitHeaderAwareClient)
	if !ok {
		return math.Inf(-1)
	}
	h, ok := aware.LastRateLimitHeaders()
	if !ok {
		return math.Inf(-1)
	}
	if h.RemainingTokens > 0 {
		return float64(h.RemainingTokens)
	}
	if h.RemainingRequests > 0 {
		return float64(h.RemainingRequests)
	}
	return 0
}

// ----------------------------------------------------------------------------
// ModelDispatchClient – dispatches to selected model
// ----------------------------------------------------------------------------

type modelDispatchClient struct {
	fallback llmclient.LLMClient
}

// NewModelDispatchClient creates a client that dispatches to the selected model.
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
