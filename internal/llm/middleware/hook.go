package llm

import (
	"context"
	"encoding/json"

	llmclient "insightify/internal/llm/client"
)

// PromptHook defines callbacks around LLM requests.
type PromptHook interface {
	Before(ctx context.Context, worker, prompt string, input any)
	After(ctx context.Context, worker string, raw json.RawMessage, err error)
}

type ctxKeyHook struct{}
type ctxKeyWorker struct{}

// WithWorker attaches a worker name to the context.
func WithWorker(ctx context.Context, worker string) context.Context {
	return context.WithValue(ctx, ctxKeyWorker{}, worker)
}

// WithPromptHook attaches a PromptHook to the context. Middlewares that call
// HookFrom(ctx) can use this to invoke Before/After around requests.
func WithPromptHook(ctx context.Context, hook PromptHook) context.Context {
	return context.WithValue(ctx, ctxKeyHook{}, hook)
}

// HookFrom returns the hook stored in the context.
func HookFrom(ctx context.Context) PromptHook {
	if v := ctx.Value(ctxKeyHook{}); v != nil {
		if h, ok := v.(PromptHook); ok {
			return h
		}
	}
	return nil
}

// WorkerFrom returns the worker string stored in the context.
func WorkerFrom(ctx context.Context) string {
	if v := ctx.Value(ctxKeyWorker{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "unknown"
}

// WithHooks calls HookFrom(ctx).Before/After around GenerateJSON.
// If no hook is present in the context, it is a no-op.
func WithHooks() Middleware {
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &hooked{next: next}
	}
}

type hooked struct{ next llmclient.LLMClient }

func (h *hooked) Name() string { return h.next.Name() }
func (h *hooked) Close() error { return h.next.Close() }
func (h *hooked) CountTokens(text string) int {
	return h.next.CountTokens(text)
}
func (h *hooked) TokenCapacity() int { return h.next.TokenCapacity() }

func (h *hooked) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if hook := HookFrom(ctx); hook != nil {
		hook.Before(ctx, WorkerFrom(ctx), prompt, input)
	}
	raw, err := h.next.GenerateJSON(ctx, prompt, input)
	if hook := HookFrom(ctx); hook != nil {
		hook.After(ctx, WorkerFrom(ctx), raw, err)
	}
	return raw, err
}

func (h *hooked) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	if hook := HookFrom(ctx); hook != nil {
		hook.Before(ctx, WorkerFrom(ctx), prompt, input)
	}
	raw, err := h.next.GenerateJSONStream(ctx, prompt, input, onChunk)
	if hook := HookFrom(ctx); hook != nil {
		hook.After(ctx, WorkerFrom(ctx), raw, err)
	}
	return raw, err
}
