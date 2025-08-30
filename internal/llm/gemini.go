package llm

import (
    "context"
    "encoding/json"
    "errors"
    "log"
    "os"
    "strconv"
    "time"

    genai "google.golang.org/genai"
)

var ErrInvalidJSON = errors.New("llm: invalid JSON from model")

// GeminiClient is a thin wrapper around the official genai client.
type GeminiClient struct {
    cli   *genai.Client
    model string
    rl    *rpsLimiter
}

func NewGeminiClient(ctx context.Context, apiKey, model string) (*GeminiClient, error) {
    cli, err := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendGeminiAPI})
    if err != nil {
        return nil, err
    }
    // Optional RPS limiter via env: LLM_RPS/GEMINI_RPS and LLM_BURST/GEMINI_BURST
    var rps float64
    var burst int
    if v := os.Getenv("LLM_RPS"); v != "" {
        if f, err := strconv.ParseFloat(v, 64); err == nil {
            rps = f
        }
    }
    if rps == 0 {
        if v := os.Getenv("GEMINI_RPS"); v != "" {
            if f, err := strconv.ParseFloat(v, 64); err == nil {
                rps = f
            }
        }
    }
    if v := os.Getenv("LLM_BURST"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            burst = n
        }
    }
    if burst == 0 {
        if v := os.Getenv("GEMINI_BURST"); v != "" {
            if n, err := strconv.Atoi(v); err == nil {
                burst = n
            }
        }
    }
    rl := newRPSLimiter(rps, burst)
    return &GeminiClient{cli: cli, model: model, rl: rl}, nil
}

func (g *GeminiClient) Name() string { return "Gemini:" + g.model }
func (g *GeminiClient) Close() error {
    if g.rl != nil {
        g.rl.Stop()
    }
    return nil
}

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
        // Respect RPS limiter per attempt (each API call consumes a token).
        if err := g.rl.Acquire(ctx); err != nil {
            lastErr = err
            break
        }
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
