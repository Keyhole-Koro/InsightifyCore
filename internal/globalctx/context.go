package globalctx

import (
	"context"
	"strings"
)

type ctxKeyGlobalContext struct{}

type ModelSelectionMode string

const (
	// ModelSelectionModePreferAvailable picks the model with the best known
	// remaining provider quota when provider/model are not explicitly pinned.
	ModelSelectionModePreferAvailable ModelSelectionMode = "prefer_available"
)

type GlobalContext struct {
	ModelSelectionMode ModelSelectionMode
	ProviderTiers      map[string]string
}

func normalizeSelectionMode(mode ModelSelectionMode) ModelSelectionMode {
	switch mode {
	case ModelSelectionModePreferAvailable:
		return mode
	default:
		return ModelSelectionModePreferAvailable
	}
}

func WithGlobalContext(ctx context.Context, gc GlobalContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	gc.ModelSelectionMode = normalizeSelectionMode(gc.ModelSelectionMode)
	if len(gc.ProviderTiers) > 0 {
		out := make(map[string]string, len(gc.ProviderTiers))
		for k, v := range gc.ProviderTiers {
			p := strings.ToLower(strings.TrimSpace(k))
			t := strings.ToLower(strings.TrimSpace(v))
			if p == "" || t == "" {
				continue
			}
			out[p] = t
		}
		gc.ProviderTiers = out
	} else {
		gc.ProviderTiers = nil
	}
	return context.WithValue(ctx, ctxKeyGlobalContext{}, gc)
}

func GlobalContextFrom(ctx context.Context) GlobalContext {
	if ctx != nil {
		if v := ctx.Value(ctxKeyGlobalContext{}); v != nil {
			if gc, ok := v.(GlobalContext); ok {
				gc.ModelSelectionMode = normalizeSelectionMode(gc.ModelSelectionMode)
				return gc
			}
		}
	}
	return GlobalContext{ModelSelectionMode: ModelSelectionModePreferAvailable}
}

func WithModelSelectionMode(ctx context.Context, mode ModelSelectionMode) context.Context {
	gc := GlobalContextFrom(ctx)
	gc.ModelSelectionMode = mode
	return WithGlobalContext(ctx, gc)
}

func ModelSelectionModeFrom(ctx context.Context) ModelSelectionMode {
	return GlobalContextFrom(ctx).ModelSelectionMode
}

func WithProviderTier(ctx context.Context, provider, tier string) context.Context {
	gc := GlobalContextFrom(ctx)
	if gc.ProviderTiers == nil {
		gc.ProviderTiers = map[string]string{}
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	t := strings.ToLower(strings.TrimSpace(tier))
	if p == "" || t == "" {
		return WithGlobalContext(ctx, gc)
	}
	gc.ProviderTiers[p] = t
	return WithGlobalContext(ctx, gc)
}

func ProviderTierFrom(ctx context.Context, provider, fallback string) string {
	gc := GlobalContextFrom(ctx)
	p := strings.ToLower(strings.TrimSpace(provider))
	if p != "" && gc.ProviderTiers != nil {
		if tier := strings.ToLower(strings.TrimSpace(gc.ProviderTiers[p])); tier != "" {
			return tier
		}
	}
	return strings.ToLower(strings.TrimSpace(fallback))
}
