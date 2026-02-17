package llmclient

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"

	genai "google.golang.org/genai"
)

// GeminiClient is a thin wrapper around the official genai client.
// It only focuses on the API call itself. Cross-cutting concerns
// (rate limiting, retries, logging, hooks) are applied via Middleware.
type GeminiClient struct {
	cli      *genai.Client
	model    string
	tokenCap int

	rlMu      sync.RWMutex
	rlLast    RateLimitHeaders
	rlHasLast bool
	rlHandler RateLimitHeaderHandler
}

func NewGeminiClient(ctx context.Context, apiKey, model string, tokenCap int) (*GeminiClient, error) {
	// NOTE: apiKey is currently unused here; the genai client may read it from env.
	// Keep the parameter for future use and to keep a consistent factory signature.
	_ = apiKey

	cli, err := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, err
	}
	if tokenCap <= 0 {
		tokenCap = 12000
	}
	return &GeminiClient{cli: cli, model: model, tokenCap: tokenCap}, nil
}

func (g *GeminiClient) Name() string { return "Gemini:" + g.model }
func (g *GeminiClient) Close() error { return nil }
func (g *GeminiClient) CountTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return CountTokens(text)
}
func (g *GeminiClient) TokenCapacity() int { return g.tokenCap }

func (g *GeminiClient) SetRateLimitHeaderHandler(handler RateLimitHeaderHandler) {
	g.rlMu.Lock()
	defer g.rlMu.Unlock()
	g.rlHandler = handler
}

func (g *GeminiClient) LastRateLimitHeaders() (RateLimitHeaders, bool) {
	g.rlMu.RLock()
	defer g.rlMu.RUnlock()
	return g.rlLast, g.rlHasLast
}

// GenerateJSON concatenates prompt and input, asks for application/json,
// and returns the model's JSON as json.RawMessage.
//
// Retries / rate limiting / logging / hooks are handled by middleware layers.
func (g *GeminiClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	full := prompt + "\n\n[INPUT JSON]\n" + string(in)

	resp, err := g.cli.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{{Parts: []*genai.Part{{Text: full}}}},
		&genai.GenerateContentConfig{ResponseMIMEType: "application/json"},
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, ErrInvalidJSON
	}
	txt := resp.Candidates[0].Content.Parts[0].Text
	return json.RawMessage(txt), nil
}

// GenerateJSONStream streams partial JSON chunks to the callback.
// Returns the final complete JSON response.
func (g *GeminiClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	full := prompt + "\n\n[INPUT JSON]\n" + string(in)

	resp, err := g.cli.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{{Parts: []*genai.Part{{Text: full}}}},
		&genai.GenerateContentConfig{ResponseMIMEType: "application/json"},
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, ErrInvalidJSON
	}
	return json.RawMessage(resp.Candidates[0].Content.Parts[0].Text), nil
}

func RegisterGeminiModels(reg ModelRegistrar) error {
	return RegisterGeminiModelsForTier(reg, "free")
}

func RegisterGeminiModelsForTier(reg ModelRegistrar, tier string) error {
	tier = normalizeTier(tier, "free")

	type geminiModel struct {
		name   string
		level  ModelLevel
		tokens int
		meta   map[string]any
		limit  *RateLimitConfig
	}
	freeLimits := &RateLimitConfig{RPM: 15, RPS: 0.25, Burst: 1}
	tier1Limits := &RateLimitConfig{RPM: 60, RPS: 1, Burst: 1}
	models := []geminiModel{}
	switch tier {
	case "tier1":
		models = append(models,
			geminiModel{name: "gemini-2.5-flash", level: ModelLevelLow, tokens: 12000, meta: map[string]any{"params": 0}, limit: tier1Limits},
			geminiModel{name: "gemini-2.5-flash", level: ModelLevelMiddle, tokens: 12000, meta: map[string]any{"params": 0}, limit: tier1Limits},
			geminiModel{name: "gemini-2.5-pro", level: ModelLevelHigh, tokens: 12000, meta: map[string]any{"params": 0}, limit: tier1Limits},
			geminiModel{name: "gemini-2.5-pro", level: ModelLevelXHigh, tokens: 12000, meta: map[string]any{"params": 0}, limit: tier1Limits},
		)
	default: // free
		models = append(models,
			geminiModel{name: "gemini-2.5-flash", level: ModelLevelLow, tokens: 12000, meta: map[string]any{"params": 0}, limit: freeLimits},
			geminiModel{name: "gemini-2.5-flash", level: ModelLevelMiddle, tokens: 12000, meta: map[string]any{"params": 0}, limit: freeLimits},
			geminiModel{name: "gemini-2.5-pro", level: ModelLevelHigh, tokens: 12000, meta: map[string]any{"params": 0}, limit: freeLimits},
			geminiModel{name: "gemini-2.5-pro", level: ModelLevelXHigh, tokens: 12000, meta: map[string]any{"params": 0}, limit: freeLimits},
		)
	}
	for _, m := range models {
		modelName := m.name
		tokens := m.tokens
		meta := m.meta
		level := m.level
		if err := reg.RegisterModel(ModelRegistration{
			Provider:  "gemini",
			Tier:      tier,
			Model:     modelName,
			Level:     level,
			MaxTokens: tokens,
			Meta:      meta,
			RateLimit: m.limit,
			Factory: func(ctx context.Context, tokenCap int) (LLMClient, error) {
				if tokenCap <= 0 {
					tokenCap = tokens
				}
				return NewGeminiClient(ctx, os.Getenv("GEMINI_API_KEY"), modelName, tokenCap)
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
