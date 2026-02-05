package llm

import (
	"context"
	"encoding/json"
)

type PromptHook interface {
	Before(ctx context.Context, worker, prompt string, input any)
	After(ctx context.Context, worker string, raw json.RawMessage, err error)
}

type ctxKeyHook struct{}
type ctxKeyWorker struct{}

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
