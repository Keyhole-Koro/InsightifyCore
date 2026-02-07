package llm

import (
	"context"
	"strings"
)

type ctxKeyModelRole struct{}
type ctxKeyModelLevel struct{}
type ctxKeyModelProvider struct{}
type ctxKeyModelName struct{}

func WithModelRole(ctx context.Context, role ModelRole) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyModelRole{}, normalizeRole(role))
}

func WithModelLevel(ctx context.Context, level ModelLevel) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKeyModelLevel{}, normalizeLevel(level))
}

func WithModelProvider(ctx context.Context, provider string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	// Context can be populated from user/env values. Normalize to canonical
	// provider keys so downstream selection is case/space insensitive.
	return context.WithValue(ctx, ctxKeyModelProvider{}, strings.ToLower(strings.TrimSpace(provider)))
}

func WithModelName(ctx context.Context, model string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	// Keep model id verbatim except outer whitespace.
	return context.WithValue(ctx, ctxKeyModelName{}, strings.TrimSpace(model))
}

func WithModelSelection(ctx context.Context, role ModelRole, level ModelLevel, provider, model string) context.Context {
	ctx = WithModelRole(ctx, role)
	ctx = WithModelLevel(ctx, level)
	ctx = WithModelProvider(ctx, provider)
	return WithModelName(ctx, model)
}

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

func ModelProviderFrom(ctx context.Context) string {
	if ctx != nil {
		if v := ctx.Value(ctxKeyModelProvider{}); v != nil {
			if provider, ok := v.(string); ok {
				// Read path also normalizes to tolerate non-normalized values
				// injected by older callers/tests.
				return strings.ToLower(strings.TrimSpace(provider))
			}
		}
	}
	return ""
}

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
