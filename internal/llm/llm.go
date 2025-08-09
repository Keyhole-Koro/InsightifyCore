package llm

import (
    "context"
    "encoding/json"
)

type LLMClient interface {
    Name() string
    GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error)
    Close() error
}