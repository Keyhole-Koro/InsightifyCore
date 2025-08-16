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

// WithHook attaches a PromptHook to the context used by GenerateJSON.
func WithHook(base LLMClient, hook PromptHook) LLMClient {
	return &hooked{base: base, hook: hook}
}

type hooked struct {
	base LLMClient
	hook PromptHook
}

func (h *hooked) Name() string { return h.base.Name() }
func (h *hooked) Close() error { return h.base.Close() }

func (h *hooked) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	ctx = context.WithValue(ctx, ctxKeyHook{}, h.hook)
	return h.base.GenerateJSON(ctx, prompt, input)
}

func WithPhase(ctx context.Context, phase string) context.Context {
	return context.WithValue(ctx, ctxKeyPhase{}, phase)
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
