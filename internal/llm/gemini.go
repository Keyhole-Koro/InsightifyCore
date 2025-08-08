package llm

import (
	"log"
	"context"
	"encoding/json"
	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  string
}

func NewGeminiClient(ctx context.Context, apiKey, model string) (*GeminiClient, error) {
    cli, err := genai.NewClient(ctx, &genai.ClientConfig{
        APIKey:  apiKey,
        Backend: genai.BackendGeminiAPI,
    })
    if err != nil {
        return nil, err
    }
    return &GeminiClient{client: cli, model: model}, nil
}


func (g *GeminiClient) Name() string {
	return "Gemini"
}

func (g *GeminiClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	fullPrompt := prompt + "\n\n[INPUT]\n" + string(in)
    log.Println("fullPrompt:", fullPrompt)
	resp, err := g.client.Models.GenerateContent(ctx, g.model,
		[]*genai.Content{{Parts: []*genai.Part{{Text: fullPrompt}}}},
		&genai.GenerateContentConfig{ResponseMIMEType: "application/json"},
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Candidates) == 0 {
		return nil, nil
	}
	return json.RawMessage(resp.Candidates[0].Content.Parts[0].Text), nil
}
