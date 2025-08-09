package llm

import (
	"context"
	"encoding/json"
)

// ---- Phase tag via context
type phaseKey struct{}

func WithPhase(ctx context.Context, phase string) context.Context {
	return context.WithValue(ctx, phaseKey{}, phase)
}
func phaseFrom(ctx context.Context) string {
	if v := ctx.Value(phaseKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ---- Hook interface
type CallHook interface {
	Before(ctx context.Context, phase string, prompt string, input any)
	After(ctx context.Context, phase string, raw json.RawMessage, err error)
}

// ---- Decorator that injects hooks without changing LLMClient interface
type hooked struct {
	base LLMClient
	hook CallHook
}

func WithHook(base LLMClient, hook CallHook) LLMClient {
	return &hooked{base: base, hook: hook}
}

func (h *hooked) Name() string { return h.base.Name() }
func (h *hooked) Close() error { return h.base.Close() }

func (h *hooked) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	if h.hook != nil {
		// best-effort; hook implementations must not panic
		h.hook.Before(ctx, phaseFrom(ctx), prompt, input)
	}
	raw, err := h.base.GenerateJSON(ctx, prompt, input)
	if h.hook != nil {
		h.hook.After(ctx, phaseFrom(ctx), raw, err)
	}
	return raw, err
}
