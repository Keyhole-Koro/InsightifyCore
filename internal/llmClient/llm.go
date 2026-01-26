package llmclient

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrInvalidJSON = errors.New("invalid json from LLM")

// PermanentError indicates an error that will not resolve with retries.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func NewPermanentError(err error) error {
	return &PermanentError{Err: err}
}

type LLMClient interface {
	Name() string
	GenerateJSON(ctx context.Context, prompt string, input any) (json.RawMessage, error)
	CountTokens(text string) int
	TokenCapacity() int
	Close() error
}