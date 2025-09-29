package llm

import (
	"context"
	"encoding/json"
)

type PromptHook interface {
	Before(ctx context.Context, phase, prompt string, input any)
	After(ctx context.Context, phase string, raw json.RawMessage, err error)
}

type ctxKeyHook struct{}
type ctxKeyPhase struct{}

func WithPhase(ctx context.Context, phase string) context.Context {
	return context.WithValue(ctx, ctxKeyPhase{}, phase)
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

// PhaseFrom returns the phase string stored in the context.
func PhaseFrom(ctx context.Context) string {
	if v := ctx.Value(ctxKeyPhase{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return "unknown"
}
