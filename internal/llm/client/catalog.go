package llmclient

import (
	"context"
	"strings"
)

type ModelLevel string

const (
	ModelLevelLow    ModelLevel = "low"
	ModelLevelMiddle ModelLevel = "middle"
	ModelLevelHigh   ModelLevel = "high"
	ModelLevelXHigh  ModelLevel = "xhigh"
)

type ClientFactory func(ctx context.Context, tokenCap int) (LLMClient, error)

type RateLimitConfig struct {
	RPM   int
	RPD   int
	TPM   int
	TPD   int
	RPS   float64
	Burst int
}

type ModelRegistration struct {
	Provider  string
	Tier      string
	Model     string
	Level     ModelLevel
	MaxTokens int
	Meta      map[string]any
	RateLimit *RateLimitConfig
	Factory   ClientFactory
}

type ModelRegistrar interface {
	RegisterModel(spec ModelRegistration) error
}

func normalizeTier(tier, fallback string) string {
	t := strings.ToLower(strings.TrimSpace(tier))
	if t == "" {
		return strings.ToLower(strings.TrimSpace(fallback))
	}
	return t
}
