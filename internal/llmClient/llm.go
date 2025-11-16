package llmclient

import (
	"context"
	"encoding/json"
)

type LLMClient interface {
	Name() string
	GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error)
	CountTokens(text string) int
	TokenCapacity() int
	Close() error
}
