package llm

import (
	"log"
    "context"
    "encoding/json"
    "errors"
    "time"

    genai "google.golang.org/genai"
)

var ErrInvalidJSON = errors.New("llm: invalid JSON returned from model")

type GeminiClient struct {
    cli   *genai.Client
    model string
}

// NewGeminiClient reads apiKey (if provided) into GOOGLE_API_KEY env and
// instantiates a Developer API client. model e.g. "gemini-2.5-flash".
func NewGeminiClient(ctx context.Context, apiKey, model string) (*GeminiClient, error) {
    cli, err := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendGeminiAPI})
    if err != nil { return nil, err }
    return &GeminiClient{cli: cli, model: model}, nil
}

func (g *GeminiClient) Name() string { return "Gemini:" + g.model }
func (g *GeminiClient) Close() error { return nil }

// GenerateJSON enforces application/json + simple retries with backoff.
func (g *GeminiClient) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
    safeInput := RedactMedia(input)

    in, _ := json.MarshalIndent(safeInput, "", "  ")
    full := prompt + "\n\n[INPUT]\n" + string(in)
	log.Printf("LLM prompt:\n%s\n", full)

    var lastErr error
    for attempt := 0; attempt < 3; attempt++ {
        resp, err := g.cli.Models.GenerateContent(ctx, g.model,
            []*genai.Content{{Parts: []*genai.Part{{Text: full}}}},
            &genai.GenerateContentConfig{ResponseMIMEType: "application/json"},
        )
        if err != nil { lastErr = err } else if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
            lastErr = ErrInvalidJSON
        } else {
            txt := resp.Candidates[0].Content.Parts[0].Text
            return json.RawMessage(txt), nil
        }
        // backoff
        time.Sleep(time.Duration(300*(1<<attempt)) * time.Millisecond)
    }
    return nil, lastErr
}
