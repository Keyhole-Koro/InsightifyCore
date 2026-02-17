package runtime

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	llmclient "insightify/internal/llm/client"
	llmmiddleware "insightify/internal/llm/middleware"
	llmmodel "insightify/internal/llm/model"
)

func newRuntimeLLMClient(ctx context.Context) (llmclient.LLMClient, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Removed globalctx usage

	reg := llmmodel.NewInMemoryModelRegistry()
	geminiTier := firstNonEmpty(strings.TrimSpace(os.Getenv("LLM_GEMINI_TIER")), "free")
	groqTier := firstNonEmpty(strings.TrimSpace(os.Getenv("LLM_GROQ_TIER")), "free")
	if err := llmclient.RegisterGeminiModelsForTier(reg, geminiTier); err != nil {
		return nil, "", err
	}
	if err := llmclient.RegisterGroqModelsForTier(reg, groqTier); err != nil {
		return nil, "", err
	}
	if err := llmmodel.RegisterFakeModels(reg); err != nil {
		return nil, "", err
	}

	tokenCap := 4096
	if raw := strings.TrimSpace(os.Getenv("LLM_TOKEN_CAP")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			tokenCap = n
		}
	}

	fallback, err := reg.BuildClient(ctx, llmmodel.ModelRoleWorker, llmmodel.ModelLevelMiddle, "", "", tokenCap)
	if err != nil {
		return nil, "", fmt.Errorf("llm fallback client failed: %w", err)
	}

	dispatch := llmmodel.NewModelDispatchClient(fallback)
	client := llmmiddleware.Wrap(dispatch,
		llmmodel.SelectModel(reg, tokenCap, llmmodel.ModelSelectionModePreferAvailable),
		llmmiddleware.RespectRateLimitSignals(llmclient.HeaderRateLimitControlAdapter{}),
		llmmiddleware.Retry(3, 300*time.Millisecond),
		llmmiddleware.WithHooks(),
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
