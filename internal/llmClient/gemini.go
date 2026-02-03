package llmclient

import (
	"context"
	"encoding/json"
	"strings"

	genai "google.golang.org/genai"
)

// GeminiClient is a thin wrapper around the official genai client.
// It only focuses on the API call itself. Cross-cutting concerns
// (rate limiting, retries, logging, hooks) are applied via Middleware.
type GeminiClient struct {
	cli      *genai.Client
	model    string
	tokenCap int
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
