package llmclient

import (
	"context"
	"encoding/json"
)

// LLMClient defines the interface for LLM providers.
type LLMClient interface {
	Name() string
	Close() error
	CountTokens(text string) int
	TokenCapacity() int
	GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error)
	// GenerateJSONStream streams partial JSON chunks to the callback.
	// Returns the final complete JSON response.
	GenerateJSONStream(ctx context.Context, prompt string, input any, onChunk func(chunk string)) (json.RawMessage, error)
}
