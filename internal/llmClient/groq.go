package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GroqClient calls the Groq Chat Completions API (OpenAI-compatible) and asks for JSON.
// See: https://console.groq.com/docs/api-reference
type GroqClient struct {
	http     *http.Client
	apiKey   string
	model    string
	baseURL  string
	tokenCap int

	rlMu      sync.RWMutex
	rlLast    RateLimitHeaders
	rlHasLast bool
	rlHandler RateLimitHeaderHandler
}

// NewGroqClient creates a Groq client. If apiKey is empty, it falls back to GROQ_API_KEY env var.
func NewGroqClient(apiKey, model string, tokenCap int) (*GroqClient, error) {
	if apiKey == "" {
		apiKey = os.Getenv("GROQ_API_KEY")
	}
	if tokenCap <= 0 {
		tokenCap = 6000
	}
	return &GroqClient{
		http:     &http.Client{Timeout: 60 * time.Second},
		apiKey:   apiKey,
		model:    model,
		baseURL:  "https://api.groq.com/openai/v1/chat/completions",
		tokenCap: tokenCap,
	}, nil
}

func (g *GroqClient) Name() string { return "Groq:" + g.model }
func (g *GroqClient) Close() error { return nil }
func (g *GroqClient) CountTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return CountTokens(text)
}
func (g *GroqClient) TokenCapacity() int { return g.tokenCap }

func (g *GroqClient) SetRateLimitHeaderHandler(handler RateLimitHeaderHandler) {
	g.rlMu.Lock()
	defer g.rlMu.Unlock()
	g.rlHandler = handler
}

func (g *GroqClient) LastRateLimitHeaders() (RateLimitHeaders, bool) {
	g.rlMu.RLock()
	defer g.rlMu.RUnlock()
	return g.rlLast, g.rlHasLast
}

type groqChatReq struct {
	Model          string            `json:"model"`
	Messages       []groqMessage     `json:"messages"`
	Temperature    float32           `json:"temperature,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}
type groqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type groqChatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// GenerateJSON assembles a single user message from prompt + input and requests JSON output.
func (g *GroqClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	userContent := "[INPUT JSON]\n" + string(in)

	reqBody := groqChatReq{
		Model: g.model,
		Messages: []groqMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: userContent},
		},
		Temperature:    0,
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	g.captureRateLimitHeaders(resp.Header)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		const max = 2048
		if len(body) > max {
			body = body[:max]
		}
		err := fmt.Errorf("groq: unexpected status %s: %s", resp.Status, string(body))
		// Check for context length exceeded (permanent error)
		if resp.StatusCode == 400 && strings.Contains(string(body), `"code":"context_length_exceeded"`) {
			return nil, NewPermanentError(err)
		}
		return nil, err
	}
	var out groqChatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return nil, ErrInvalidJSON
	}
	// Ensure the content is valid JSON; if not, wrap with an error
	raw := json.RawMessage(out.Choices[0].Message.Content)
	var scratch any
	if err := json.Unmarshal(raw, &scratch); err != nil {
		return nil, ErrInvalidJSON
	}
	return raw, nil
}

func (g *GroqClient) captureRateLimitHeaders(h http.Header) {
	parsed, ok := parseGroqRateLimitHeaders(h)
	if !ok {
		return
	}
	g.rlMu.Lock()
	g.rlLast = parsed
	g.rlHasLast = true
	handler := g.rlHandler
	g.rlMu.Unlock()
	if handler != nil {
		handler(parsed)
	}
}

func (g *GroqClient) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	raw, err := g.GenerateJSON(ctx, prompt, input)
	if err != nil {
		return nil, err
	}
	if onChunk != nil {
		onChunk(string(raw))
	}
	return raw, nil
}

func RegisterGroqModels(reg ModelRegistrar) error {
	return RegisterGroqModelsForTier(reg, "free")
}

func RegisterGroqModelsForTier(reg ModelRegistrar, tier string) error {
	tier = normalizeTier(tier, "free")

	type groqModel struct {
		name   string
		level  ModelLevel
		tokens int
		meta   map[string]any
		limit  *RateLimitConfig
	}
	// Base limits are sourced from Groq rate-limit docs and used as defaults.
	// See: https://console.groq.com/docs/rate-limits
	// Note: limits can vary by account/tier. These values are hints, not guarantees.
	models := []groqModel{
		{name: "allam-2-7b", level: ModelLevelLow, tokens: 6000, meta: map[string]any{"params": 7_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 7_000, TPM: 6_000, TPD: 500_000}},
		{name: "groq/compound", level: ModelLevelHigh, tokens: 6000, meta: map[string]any{"params": 0}, limit: &RateLimitConfig{RPM: 15, RPD: 200}},
		{name: "groq/compound-mini", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 0}, limit: &RateLimitConfig{RPM: 15, RPD: 200}},
		{name: "llama-3.1-8b-instant", level: ModelLevelLow, tokens: 6000, meta: map[string]any{"params": 8_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 14_400, TPM: 6_000, TPD: 500_000}},
		{name: "llama-3.3-70b-versatile", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 70_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 12_000, TPD: 100_000}},
		{name: "llama-3.3-70b-versatile", level: ModelLevelHigh, tokens: 6000, meta: map[string]any{"params": 70_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 12_000, TPD: 100_000}},
		{name: "llama-3.3-70b-versatile", level: ModelLevelXHigh, tokens: 6000, meta: map[string]any{"params": 70_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 12_000, TPD: 100_000}},
		{name: "meta-llama/llama-4-maverick-17b-128e-instruct", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 17_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 6_000, TPD: 500_000}},
		{name: "meta-llama/llama-4-scout-17b-16e-instruct", level: ModelLevelHigh, tokens: 6000, meta: map[string]any{"params": 17_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 30_000, TPD: 500_000}},
		{name: "meta-llama/llama-guard-4-12b", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 12_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 14_400, TPM: 15_000, TPD: 500_000}},
		{name: "meta-llama/llama-prompt-guard-2-22m", level: ModelLevelLow, tokens: 6000, meta: map[string]any{"params": 22_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 14_400, TPM: 15_000, TPD: 500_000}},
		{name: "meta-llama/llama-prompt-guard-2-86m", level: ModelLevelLow, tokens: 6000, meta: map[string]any{"params": 86_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 14_400, TPM: 15_000, TPD: 500_000}},
		{name: "moonshotai/kimi-k2-instruct", level: ModelLevelHigh, tokens: 6000, meta: map[string]any{"params": 0}, limit: &RateLimitConfig{RPM: 60, RPD: 1_000, TPM: 10_000, TPD: 300_000}},
		{name: "moonshotai/kimi-k2-instruct-0905", level: ModelLevelHigh, tokens: 6000, meta: map[string]any{"params": 0}, limit: &RateLimitConfig{RPM: 60, RPD: 1_000, TPM: 10_000, TPD: 300_000}},
		{name: "openai/gpt-oss-120b", level: ModelLevelXHigh, tokens: 6000, meta: map[string]any{"params": 120_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 8_000, TPD: 200_000}},
		{name: "openai/gpt-oss-20b", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 20_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 8_000, TPD: 200_000}},
		{name: "openai/gpt-oss-safeguard-20b", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 20_000_000_000}, limit: &RateLimitConfig{RPM: 30, RPD: 1_000, TPM: 8_000, TPD: 200_000}},
		{name: "qwen/qwen3-32b", level: ModelLevelMiddle, tokens: 6000, meta: map[string]any{"params": 32_000_000_000}, limit: &RateLimitConfig{RPM: 60, RPD: 1_000, TPM: 6_000, TPD: 500_000}},
		{name: "whisper-large-v3", level: ModelLevelLow, tokens: 6000, meta: map[string]any{"params": 0, "modality": "audio"}, limit: &RateLimitConfig{RPM: 20, RPD: 2_000}},
		{name: "whisper-large-v3-turbo", level: ModelLevelLow, tokens: 6000, meta: map[string]any{"params": 0, "modality": "audio"}, limit: &RateLimitConfig{RPM: 20, RPD: 2_000}},
	}

	boostLimitForTier := func(in *RateLimitConfig) *RateLimitConfig {
		if in == nil {
			return nil
		}
		out := *in
		if tier == "developer" {
			out.RPM *= 3
			out.RPD *= 3
			out.TPM *= 3
			out.TPD *= 3
			if out.RPS > 0 {
				out.RPS = out.RPS * 3
			}
			if out.Burst > 0 {
				out.Burst = out.Burst * 2
			}
		}
		return &out
	}

	for _, m := range models {
		modelName := m.name
		tokens := m.tokens
		meta := m.meta
		level := m.level
		if err := reg.RegisterModel(ModelRegistration{
			Provider:  "groq",
			Tier:      tier,
			Model:     modelName,
			Level:     level,
			MaxTokens: tokens,
			Meta:      meta,
			RateLimit: boostLimitForTier(m.limit),
			Factory: func(ctx context.Context, tokenCap int) (LLMClient, error) {
				_ = ctx
				if tokenCap <= 0 {
					tokenCap = tokens
				}
				return NewGroqClient(os.Getenv("GROQ_API_KEY"), modelName, tokenCap)
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

// parseGroqRateLimitHeaders parses Groq-specific rate-limit response headers.
// Groq semantics:
// - request fields are RPD
// - token fields are TPM
func parseGroqRateLimitHeaders(h http.Header) (RateLimitHeaders, bool) {
	out := RateLimitHeaders{}
	found := false

	readInt := func(key string) (int, bool) {
		v := strings.TrimSpace(h.Get(key))
		if v == "" {
			return 0, false
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	readDur := func(key string) (time.Duration, bool) {
		v := strings.TrimSpace(h.Get(key))
		if v == "" {
			return 0, false
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, false
		}
		return d, true
	}

	if v, ok := readInt("retry-after"); ok {
		out.RetryAfterSeconds = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-limit-requests"); ok {
		out.LimitRequests = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-limit-tokens"); ok {
		out.LimitTokens = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-remaining-requests"); ok {
		out.RemainingRequests = v
		found = true
	}
	if v, ok := readInt("x-ratelimit-remaining-tokens"); ok {
		out.RemainingTokens = v
		found = true
	}
	if v, ok := readDur("x-ratelimit-reset-requests"); ok {
		out.ResetRequests = v
		found = true
	}
	if v, ok := readDur("x-ratelimit-reset-tokens"); ok {
		out.ResetTokens = v
		found = true
	}

	return out, found
}
