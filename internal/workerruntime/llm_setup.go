package runtime

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"insightify/internal/globalctx"
	"insightify/internal/llm"
	llmclient "insightify/internal/llmClient"
)

func newRuntimeLLMClient(ctx context.Context) (llmclient.LLMClient, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = globalctx.WithGlobalContext(ctx, globalctx.GlobalContext{
		ModelSelectionMode: globalctx.ModelSelectionModePreferAvailable,
		ProviderTiers: map[string]string{
			"gemini": strings.TrimSpace(os.Getenv("LLM_GEMINI_TIER")),
			"groq":   strings.TrimSpace(os.Getenv("LLM_GROQ_TIER")),
		},
	})

	reg := llm.NewInMemoryModelRegistry()
	geminiTier := firstNonEmpty(strings.TrimSpace(os.Getenv("LLM_GEMINI_TIER")), globalctx.ProviderTierFrom(ctx, "gemini", "free"))
	groqTier := firstNonEmpty(strings.TrimSpace(os.Getenv("LLM_GROQ_TIER")), globalctx.ProviderTierFrom(ctx, "groq", "free"))
	if err := llmclient.RegisterGeminiModelsForTier(reg, geminiTier); err != nil {
		return nil, "", err
	}
	if err := llmclient.RegisterGroqModelsForTier(reg, groqTier); err != nil {
		return nil, "", err
	}
	if err := llm.RegisterFakeModels(reg); err != nil {
		return nil, "", err
	}

	tokenCap := 4096
	if raw := strings.TrimSpace(os.Getenv("LLM_TOKEN_CAP")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			tokenCap = n
		}
	}

	fallback, err := reg.BuildClient(ctx, llm.ModelRoleWorker, llm.ModelLevelMiddle, "", "", tokenCap)
	if err != nil {
		return nil, "", fmt.Errorf("llm fallback client failed: %w", err)
	}

	dispatch := llm.NewModelDispatchClient(fallback)
	client := llm.Wrap(dispatch,
		llm.SelectModel(reg, tokenCap),
		llm.RespectRateLimitSignals(llmclient.HeaderRateLimitControlAdapter{}),
		llm.Retry(3, 300*time.Millisecond),
		llm.WithHooks(),
	)
	modelSalt := strings.TrimSpace(os.Getenv("CACHE_SALT")) + "|" + reg.DefaultsSalt()
	return client, modelSalt, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
