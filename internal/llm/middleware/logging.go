package llm

import (
	"context"
	"encoding/json"
	"log"

	llmclient "insightify/internal/llm/client"
)

// WithLogging logs request size and errors. Provide a custom logger or nil
// to use log.Default().
func WithLogging(logger *log.Logger) Middleware {
	if logger == nil {
		logger = log.Default()
	}
	return func(next llmclient.LLMClient) llmclient.LLMClient {
		return &logging{next: next, log: logger}
	}
}

type logging struct {
	next llmclient.LLMClient
	log  *log.Logger
}

func (l *logging) Name() string { return l.next.Name() }
func (l *logging) Close() error { return l.next.Close() }
func (l *logging) CountTokens(text string) int {
	return l.next.CountTokens(text)
}
func (l *logging) TokenCapacity() int { return l.next.TokenCapacity() }

func (l *logging) GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	l.log.Printf("LLM request (%s): %d bytes", WorkerFrom(ctx), len(prompt)+len(in))
	raw, err := l.next.GenerateJSON(ctx, prompt, input)
	if err != nil {
		l.log.Printf("LLM error (%s): %v", WorkerFrom(ctx), err)
	}
	return raw, err
}

func (l *logging) GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error) {
	in, _ := json.MarshalIndent(input, "", "  ")
	l.log.Printf("LLM stream request (%s): %d bytes", WorkerFrom(ctx), len(prompt)+len(in))
	raw, err := l.next.GenerateJSONStream(ctx, prompt, input, onChunk)
	if err != nil {
		l.log.Printf("LLM stream error (%s): %v", WorkerFrom(ctx), err)
	}
	return raw, err
}
