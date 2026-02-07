package globalctx

import (
	"context"
	"testing"
)

func TestProviderTierFrom_DefaultAndOverride(t *testing.T) {
	ctx := context.Background()
	if got := ProviderTierFrom(ctx, "groq", "free"); got != "free" {
		t.Fatalf("fallback tier: got=%s want=free", got)
	}

	ctx = WithGlobalContext(ctx, GlobalContext{
		ModelSelectionMode: ModelSelectionModePreferAvailable,
		ProviderTiers: map[string]string{
			" GROQ ": " Developer ",
		},
	})
	if got := ProviderTierFrom(ctx, "groq", "free"); got != "developer" {
		t.Fatalf("context tier: got=%s want=developer", got)
	}
	if got := ProviderTierFrom(ctx, "gemini", "free"); got != "free" {
		t.Fatalf("unknown provider fallback: got=%s want=free", got)
	}
}
