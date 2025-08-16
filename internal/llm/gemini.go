package llm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	genai "google.golang.org/genai"
)

var ErrInvalidJSON = errors.New("llm: invalid JSON from model")

// GeminiClient is a thin wrapper around the official genai client.
type GeminiClient struct {
	cli   *genai.Client
	model string
}

func NewGeminiClient(ctx context.Context, apiKey, model string) (*GeminiClient, error) {
	cli, err := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, err
	}
	return &GeminiClient{cli: cli, model: model}, nil
}

func (g *GeminiClient) Name() string { return "Gemini:" + g.model }
func (g *GeminiClient) Close() error { return nil }

// GenerateJSON sends the concatenated prompt/input and requests application/json.
func (g *GeminiClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	phase := PhaseFrom(ctx)
	if hook := HookFrom(ctx); hook != nil {
		hook.Before(ctx, phase, prompt, input)
	}

	in, _ := json.MarshalIndent(input, "", "  ")
	full := prompt + "\n\n[INPUT JSON]\n" + string(in)
	log.Printf("LLM request (%s): %d bytes", phase, len(full))

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := g.cli.Models.GenerateContent(ctx, g.model,
			[]*genai.Content{{Parts: []*genai.Part{{Text: full}}}},
			&genai.GenerateContentConfig{ResponseMIMEType: "application/json"},
		)
		if err != nil {
			lastErr = err
		} else if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			lastErr = ErrInvalidJSON
		} else {
			txt := resp.Candidates[0].Content.Parts[0].Text
			raw := json.RawMessage(txt)
			if hook := HookFrom(ctx); hook != nil {
				hook.After(ctx, phase, raw, nil)
			}
			return raw, nil
		}
		time.Sleep(time.Duration(300*(1<<attempt)) * time.Millisecond)
	}
	if hook := HookFrom(ctx); hook != nil {
		hook.After(ctx, phase, nil, lastErr)
	}
	return nil, lastErr
}
